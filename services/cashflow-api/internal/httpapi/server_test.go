package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/cashflow/desktop/api/internal/application"
	"github.com/cashflow/desktop/api/internal/httpapi"
	"github.com/cashflow/desktop/api/internal/infrastructure/sqlite"
	"log/slog"
)

func TestDeactivateCategoryRoute(t *testing.T) {
	repo, err := sqlite.Open(filepath.Join(t.TempDir(), "cashflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	app := application.New(repo)
	ctx := context.Background()
	if _, err = app.Initialize(ctx, "admin@example.com", "Admin", "SecureAdmin123", "route-test"); err != nil {
		t.Fatal(err)
	}
	token, _, err := app.Login(ctx, "admin@example.com", "SecureAdmin123")
	if err != nil {
		t.Fatal(err)
	}
	categories, err := repo.ListCategories(ctx)
	if err != nil || len(categories) == 0 {
		t.Fatalf("categories: %v", err)
	}

	server := httptest.NewServer(httpapi.New(app, slog.Default()).Handler())
	defer server.Close()
	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/categories/"+categories[0].ID+"/deactivate", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", response.StatusCode)
	}
	active, err := repo.ListCategories(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != len(categories)-1 {
		t.Fatalf("active categories = %d", len(active))
	}
}

func TestImportTransactionsAndTemplateRoute(t *testing.T) {
	repo, err := sqlite.Open(filepath.Join(t.TempDir(), "cashflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	app := application.New(repo)
	ctx := context.Background()
	if _, err = app.Initialize(ctx, "admin@example.com", "Admin", "SecureAdmin123", "import-test"); err != nil {
		t.Fatal(err)
	}
	token, admin, err := app.Login(ctx, "admin@example.com", "SecureAdmin123")
	if err != nil {
		t.Fatal(err)
	}
	if err = app.CreateAccount(ctx, admin, "Cash", "cash", "import-test"); err != nil {
		t.Fatal(err)
	}
	if err = app.CreateCategory(ctx, admin, "Supplies", "expense", "import-test"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(app, slog.Default()).Handler())
	defer server.Close()

	templateRequest, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/templates/transactions.csv", nil)
	templateRequest.Header.Set("Authorization", "Bearer "+token)
	templateResponse, err := http.DefaultClient.Do(templateRequest)
	if err != nil {
		t.Fatal(err)
	}
	if templateResponse.StatusCode != http.StatusOK {
		t.Fatalf("template status = %d", templateResponse.StatusCode)
	}
	templateResponse.Body.Close()

	payload, _ := json.Marshal(map[string]string{"csv": "account_name,category_name,direction,amount_minor,currency,description,occurred_on\nCash,Supplies,expense,125000,ARS,Printer supplies,2026-08-22\n"})
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/imports/transactions", bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("import status = %d", response.StatusCode)
	}
	transactions, err := repo.ListTransactions(ctx, "", "")
	if err != nil || len(transactions) != 1 {
		t.Fatalf("transactions = %#v, err = %v", transactions, err)
	}
	if transactions[0].AmountMinor != 125000 || transactions[0].Direction != "expense" {
		t.Fatalf("transaction = %#v", transactions[0])
	}
}

func TestActivateUserRoute(t *testing.T) {
	repo, err := sqlite.Open(filepath.Join(t.TempDir(), "cashflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	app := application.New(repo)
	ctx := context.Background()
	if _, err = app.Initialize(ctx, "admin@example.com", "Admin", "SecureAdmin123", "activate-route"); err != nil {
		t.Fatal(err)
	}
	token, admin, err := app.Login(ctx, "admin@example.com", "SecureAdmin123")
	if err != nil {
		t.Fatal(err)
	}
	if err = app.CreateUser(ctx, admin, "operator@example.com", "Operator", "another secure password", "operator", "activate-route"); err != nil {
		t.Fatal(err)
	}
	users, err := repo.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	operator := users[1]
	if err = app.DeactivateUser(ctx, admin, operator.ID, "activate-route"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(app, slog.Default()).Handler())
	defer server.Close()
	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/users/"+operator.ID+"/activate", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

func TestLogsRouteReturnsRecentRequestsForAdministrator(t *testing.T) {
	repo, err := sqlite.Open(filepath.Join(t.TempDir(), "cashflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	app := application.New(repo)
	ctx := context.Background()
	if _, err = app.Initialize(ctx, "admin@example.com", "Admin", "SecureAdmin123", "logs-route"); err != nil {
		t.Fatal(err)
	}
	token, _, err := app.Login(ctx, "admin@example.com", "SecureAdmin123")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(app, slog.Default()).Handler())
	defer server.Close()

	health, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	health.Body.Close()

	request, err := http.NewRequest(http.MethodGet, server.URL+"/v1/logs", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	var events []struct {
		StatusCode int    `json:"status_code"`
		Operation  string `json:"operation"`
	}
	if err := json.NewDecoder(response.Body).Decode(&events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].StatusCode != http.StatusOK || events[0].Operation != "GET /health" {
		t.Fatalf("events = %#v", events)
	}
}
