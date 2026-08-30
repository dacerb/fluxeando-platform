package application_test

import (
	"context"
	"github.com/cashflow/desktop/api/internal/application"
	"github.com/cashflow/desktop/api/internal/domain"
	"github.com/cashflow/desktop/api/internal/infrastructure/sqlite"
	"path/filepath"
	"testing"
)

func TestOnboardingLoginTransactionAndAudit(t *testing.T) {
	repo, e := sqlite.Open(filepath.Join(t.TempDir(), "cashflow.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer repo.Close()
	app := application.New(repo)
	ctx := context.Background()
	code, e := app.Initialize(ctx, "admin@example.com", "Admin", "SecureAdmin123", "correlation-test")
	if e != nil || code == "" {
		t.Fatalf("initialize: %v", e)
	}
	token, admin, e := app.Login(ctx, "admin@example.com", "SecureAdmin123")
	if e != nil || token == "" {
		t.Fatalf("login: %v", e)
	}
	if e = app.CreateAccount(ctx, admin, "Cash", "cash", "correlation-test"); e != nil {
		t.Fatal(e)
	}
	accounts, e := repo.ListAccounts(ctx)
	if e != nil || len(accounts) != 1 {
		t.Fatalf("accounts: %v", e)
	}
	if e = app.CreateTransaction(ctx, admin, domain.Transaction{AccountID: accounts[0].ID, Direction: "expense", AmountMinor: 1234, Currency: "ARS", Description: "Lunch", OccurredOn: "2026-08-22"}, "correlation-test"); e != nil {
		t.Fatal(e)
	}
	transactions, e := repo.ListTransactions(ctx, "", "")
	if e != nil || len(transactions) != 1 {
		t.Fatalf("transactions: %v", e)
	}
	if e = app.VoidTransaction(ctx, admin, transactions[0].ID, "", "correlation-test"); e != nil {
		t.Fatal(e)
	}
	transactions, _ = repo.ListTransactions(ctx, "", "")
	if transactions[0].Status != "voided" {
		t.Fatalf("status = %s", transactions[0].Status)
	}
}

func TestInitialAdministratorValidatesEmailNameAndAlphanumericPassword(t *testing.T) {
	tests := []struct{ email, name, password string }{
		{"not-an-email", "Admin", "SecureAdmin123"},
		{"admin@example.com", "", "SecureAdmin123"},
		{"admin@example.com", "Admin", "short123"},
		{"admin@example.com", "Admin", "Secure\nAdmin123"},
		{"admin@example.com", "Admin", "Secure Admin123"},
	}
	for _, test := range tests {
		repo, err := sqlite.Open(filepath.Join(t.TempDir(), "cashflow.db"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = application.New(repo).Initialize(context.Background(), test.email, test.name, test.password, "validation"); err == nil {
			t.Fatalf("expected invalid initial administrator to fail: %#v", test)
		}
		_ = repo.Close()
	}
}

func TestRememberedSessionSurvivesServiceRestartAndCanBeRevoked(t *testing.T) {
	repo, err := sqlite.Open(filepath.Join(t.TempDir(), "cashflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	ctx := context.Background()
	first := application.New(repo)
	if _, err = first.Initialize(ctx, "admin@example.com", "Admin", "SecureAdmin123", "c"); err != nil {
		t.Fatal(err)
	}
	token, user, err := first.LoginRemembered(ctx, "admin@example.com", "SecureAdmin123")
	if err != nil || token == "" || user.ID == "" {
		t.Fatalf("remembered login: %v", err)
	}
	second := application.New(repo)
	if restored, ok := second.Session(token); !ok || restored.ID != user.ID {
		t.Fatal("remembered session was not restored after service restart")
	}
	second.Logout(token)
	if _, ok := application.New(repo).Session(token); ok {
		t.Fatal("remembered session remained valid after logout")
	}
}

func TestRecoveryCodeCanOnlyBeUsedOnce(t *testing.T) {
	repo, e := sqlite.Open(filepath.Join(t.TempDir(), "cashflow.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer repo.Close()
	app := application.New(repo)
	ctx := context.Background()
	code, e := app.Initialize(ctx, "admin@example.com", "Admin", "SecureAdmin123", "c")
	if e != nil {
		t.Fatal(e)
	}
	nextCode, e := app.Recover(ctx, "admin@example.com", code, "AnotherAdmin123", "c")
	if e != nil || nextCode == "" || nextCode == code {
		t.Fatal(e)
	}
	if _, _, e = app.Login(ctx, "admin@example.com", "AnotherAdmin123"); e != nil {
		t.Fatal(e)
	}
	if _, e = app.Recover(ctx, "admin@example.com", code, "ThirdAdmin123", "c"); e == nil {
		t.Fatal("recovery code should be consumed")
	}
	if _, e = app.Recover(ctx, "admin@example.com", nextCode, "ThirdAdmin123", "c"); e != nil {
		t.Fatalf("rotated recovery code should work: %v", e)
	}
}

func TestAdministratorWithoutRecoveryCodeReceivesOneOnLogin(t *testing.T) {
	repo, err := sqlite.Open(filepath.Join(t.TempDir(), "cashflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	app := application.New(repo)
	ctx := context.Background()
	if _, err = app.Initialize(ctx, "admin@example.com", "Admin", "SecureAdmin123", "login-recovery"); err != nil {
		t.Fatal(err)
	}
	_, admin, err := app.Login(ctx, "admin@example.com", "SecureAdmin123")
	if err != nil {
		t.Fatal(err)
	}
	_, passwordHash, _, err := repo.UserCredentials(ctx, admin.Email)
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.UpdatePassword(ctx, admin.ID, passwordHash, false); err != nil {
		t.Fatal(err)
	}
	code, err := app.EnsureRecoveryCode(ctx, admin, "login-recovery")
	if err != nil || code == "" {
		t.Fatalf("recovery code was not issued: %v", err)
	}
	if duplicate, err := app.EnsureRecoveryCode(ctx, admin, "login-recovery"); err != nil || duplicate != "" {
		t.Fatalf("recovery code should not be reissued while active: %q, %v", duplicate, err)
	}
	if _, err = app.Recover(ctx, admin.Email, code, "AnotherAdmin123", "login-recovery"); err != nil {
		t.Fatalf("issued recovery code should work: %v", err)
	}
}

func TestAccountsAndCategoriesArePersistedAndRoleProtected(t *testing.T) {
	repo, e := sqlite.Open(filepath.Join(t.TempDir(), "cashflow.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer repo.Close()
	app := application.New(repo)
	ctx := context.Background()
	_, e = app.Initialize(ctx, "admin@example.com", "Admin", "SecureAdmin123", "correlation-test")
	if e != nil {
		t.Fatal(e)
	}
	_, admin, e := app.Login(ctx, "admin@example.com", "SecureAdmin123")
	if e != nil {
		t.Fatal(e)
	}
	if e = app.CreateAccount(ctx, admin, "Operating account", "bank", "correlation-test"); e != nil {
		t.Fatal(e)
	}
	accounts, e := repo.ListAccounts(ctx)
	if e != nil || len(accounts) != 1 || accounts[0].Name != "Operating account" {
		t.Fatalf("accounts = %#v, err = %v", accounts, e)
	}
	if e = app.CreateCategory(ctx, admin, "Supplies", "expense", "correlation-test"); e != nil {
		t.Fatal(e)
	}
	categories, e := repo.ListCategories(ctx)
	if e != nil || len(categories) != 3 {
		t.Fatalf("categories = %#v, err = %v", categories, e)
	}
	operator := domain.User{ID: "operator", Role: string(domain.RoleOperator)}
	if e = app.CreateAccount(ctx, operator, "Unauthorized", "cash", "correlation-test"); e == nil {
		t.Fatal("operator must not create accounts")
	}
}

func TestAccountAndCategoryLifecycleIsAudited(t *testing.T) {
	repo, e := sqlite.Open(filepath.Join(t.TempDir(), "cashflow.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer repo.Close()
	app := application.New(repo)
	ctx := context.Background()
	if _, e = app.Initialize(ctx, "admin@example.com", "Admin", "SecureAdmin123", "lifecycle"); e != nil {
		t.Fatal(e)
	}
	_, admin, e := app.Login(ctx, "admin@example.com", "SecureAdmin123")
	if e != nil {
		t.Fatal(e)
	}
	if e = app.CreateAccount(ctx, admin, "Petty cash", "cash", "lifecycle"); e != nil {
		t.Fatal(e)
	}
	accounts, _ := repo.ListAccounts(ctx)
	if e = app.UpdateAccount(ctx, admin, accounts[0].ID, "Office cash", "wallet", "lifecycle"); e != nil {
		t.Fatal(e)
	}
	updated, _ := repo.Account(ctx, accounts[0].ID)
	if updated.Name != "Office cash" || updated.Type != "wallet" {
		t.Fatalf("account = %#v", updated)
	}
	if e = app.DeactivateAccount(ctx, admin, updated.ID, "lifecycle"); e != nil {
		t.Fatal(e)
	}
	active, _ := repo.ListAccounts(ctx)
	if len(active) != 0 {
		t.Fatalf("active accounts = %#v", active)
	}

	operator := domain.User{ID: "operator", Role: string(domain.RoleOperator)}
	manager := domain.User{ID: "manager", Role: string(domain.RoleManager)}
	categories, _ := repo.ListCategories(ctx)
	if e = app.UpdateCategory(ctx, manager, categories[0].ID, "Revenue", "income", "lifecycle"); e != nil {
		t.Fatal(e)
	}
	if e = app.DeactivateCategory(ctx, manager, categories[0].ID, "", "lifecycle"); e != nil {
		t.Fatal(e)
	}
	var events int
	if e = repo.DB.QueryRowContext(ctx, "SELECT count(*) FROM audit_events WHERE correlation_id='lifecycle'").Scan(&events); e != nil {
		t.Fatal(e)
	}
	if events < 6 {
		t.Fatalf("expected lifecycle audit events, got %d", events)
	}
	if e = app.UpdateAccount(ctx, operator, updated.ID, "No", "cash", "lifecycle"); e == nil {
		t.Fatal("operator must not update accounts")
	}
	if e = app.UpdateCategory(ctx, operator, categories[1].ID, "No", "expense", "lifecycle"); e == nil {
		t.Fatal("operator must not update categories")
	}
}

func TestManagerAndOperatorVoidRequestPermissions(t *testing.T) {
	repo, e := sqlite.Open(filepath.Join(t.TempDir(), "cashflow.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer repo.Close()
	app := application.New(repo)
	ctx := context.Background()
	if _, e = app.Initialize(ctx, "admin@example.com", "Admin", "SecureAdmin123", "roles"); e != nil {
		t.Fatal(e)
	}
	_, admin, e := app.Login(ctx, "admin@example.com", "SecureAdmin123")
	if e != nil {
		t.Fatal(e)
	}
	if e = app.CreateUser(ctx, admin, "manager@example.com", "Manager", "another secure password", "manager", "roles"); e != nil {
		t.Fatal(e)
	}
	if e = app.CreateUser(ctx, admin, "operator@example.com", "Operator", "another secure password", "operator", "roles"); e != nil {
		t.Fatal(e)
	}
	_, manager, e := app.Login(ctx, "manager@example.com", "another secure password")
	if e != nil {
		t.Fatal(e)
	}
	_, operator, e := app.Login(ctx, "operator@example.com", "another secure password")
	if e != nil {
		t.Fatal(e)
	}
	if e = app.CreateAccount(ctx, manager, "Working cash", "cash", "roles"); e != nil {
		t.Fatal(e)
	}
	accounts, _ := repo.ListAccounts(ctx)
	if e = app.CreateTransaction(ctx, operator, domain.Transaction{AccountID: accounts[0].ID, Direction: "expense", AmountMinor: 100, Currency: "ARS", Description: "Test", OccurredOn: "2026-08-22"}, "roles"); e != nil {
		t.Fatal(e)
	}
	transactions, _ := repo.ListTransactionsByCreator(ctx, "", "", operator.ID)
	if len(transactions) != 1 {
		t.Fatal("operator movement was not persisted")
	}
	if e = app.VoidTransaction(ctx, operator, transactions[0].ID, "duplicate expense", "roles"); e != nil {
		t.Fatal(e)
	}
	requests, e := repo.ListDeletionRequests(ctx)
	if e != nil || len(requests) != 1 || requests[0].Status != "pending" || requests[0].Reason != "duplicate expense" {
		t.Fatalf("requests = %#v, err = %v", requests, e)
	}
	if e = app.VoidTransaction(ctx, operator, transactions[0].ID, "again", "roles"); e == nil {
		t.Fatal("only one pending deletion request is allowed")
	}
	if e = app.CancelDeletionRequest(ctx, operator, requests[0].ID, "roles"); e != nil {
		t.Fatal(e)
	}
	if e = app.VoidTransaction(ctx, operator, transactions[0].ID, "corrected reason", "roles"); e != nil {
		t.Fatal(e)
	}
	requests, e = repo.ListDeletionRequests(ctx)
	if e != nil || len(requests) != 2 || requests[0].Status != "pending" {
		t.Fatalf("requests after cancellation = %#v, err = %v", requests, e)
	}
	if e = app.ResolveDeletionRequest(ctx, manager, requests[0].ID, "approved", "roles"); e != nil {
		t.Fatal(e)
	}
	all, _ := repo.ListTransactions(ctx, "", "")
	if all[0].Status != "voided" {
		t.Fatalf("status = %s", all[0].Status)
	}
	if e = app.CreateUser(ctx, manager, "blocked@example.com", "Blocked", "another secure password", "operator", "roles"); e == nil {
		t.Fatal("manager must not administer users")
	}
}

func TestAdministratorCanDeactivateAndDeleteUsers(t *testing.T) {
	repo, err := sqlite.Open(filepath.Join(t.TempDir(), "cashflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	app := application.New(repo)
	ctx := context.Background()
	if _, err = app.Initialize(ctx, "admin@example.com", "Admin", "SecureAdmin123", "users"); err != nil {
		t.Fatal(err)
	}
	_, admin, err := app.Login(ctx, "admin@example.com", "SecureAdmin123")
	if err != nil {
		t.Fatal(err)
	}
	if err = app.CreateUser(ctx, admin, "operator@example.com", "Operator", "another secure password", "operator", "users"); err != nil {
		t.Fatal(err)
	}
	users, err := repo.ListUsers(ctx)
	if err != nil || len(users) != 2 {
		t.Fatalf("users = %#v, err = %v", users, err)
	}
	operator := users[1]
	if err = app.DeactivateUser(ctx, admin, operator.ID, "users"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = app.Login(ctx, "operator@example.com", "another secure password"); err == nil {
		t.Fatal("deactivated user must not be able to sign in")
	}
	if err = app.DeleteUser(ctx, admin, operator.ID, "users"); err != nil {
		t.Fatal(err)
	}
	users, err = repo.ListUsers(ctx)
	if err != nil || len(users) != 1 {
		t.Fatalf("users after deletion = %#v, err = %v", users, err)
	}
	if err = app.DeactivateUser(ctx, admin, admin.ID, "users"); err == nil {
		t.Fatal("administrator must not deactivate their own user")
	}
}

func TestImportTransactionsCreatesMissingCategories(t *testing.T) {
	repo, err := sqlite.Open(filepath.Join(t.TempDir(), "cashflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	app := application.New(repo)
	ctx := context.Background()
	if _, err = app.Initialize(ctx, "admin@example.com", "Admin", "SecureAdmin123", "import"); err != nil {
		t.Fatal(err)
	}
	_, admin, err := app.Login(ctx, "admin@example.com", "SecureAdmin123")
	if err != nil {
		t.Fatal(err)
	}
	if err = app.CreateAccount(ctx, admin, "Cash", "cash", "import"); err != nil {
		t.Fatal(err)
	}
	if err = app.ImportTransactions(ctx, admin, []application.TransactionImport{{AccountName: "Cash", CategoryName: "New income", Direction: "income", AmountMinor: 1500, Currency: "ARS", Description: "Imported", OccurredOn: "2026-08-23"}}, "import"); err != nil {
		t.Fatal(err)
	}
	category, err := repo.CategoryByName(ctx, "New income")
	if err != nil || category.Direction != "income" {
		t.Fatalf("category = %#v, err = %v", category, err)
	}
	transactions, err := repo.ListTransactions(ctx, "", "")
	if err != nil || len(transactions) != 1 || transactions[0].CategoryID != category.ID {
		t.Fatalf("transactions = %#v, err = %v", transactions, err)
	}
}
