package httpapi

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/cashflow/desktop/api/internal/application"
	"github.com/cashflow/desktop/api/internal/domain"
	"github.com/google/uuid"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	App *application.Service
	Log *slog.Logger
}

func New(app *application.Service, log *slog.Logger) *Server { return &Server{app, log} }
func (s *Server) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /health", s.health)
	m.HandleFunc("GET /v1/setup", s.setup)
	m.HandleFunc("POST /v1/setup/initialize", s.initialize)
	m.HandleFunc("POST /v1/auth/login", s.login)
	m.HandleFunc("POST /v1/auth/logout", s.logout)
	m.HandleFunc("POST /v1/auth/recover", s.recover)
	m.HandleFunc("GET /v1/users", s.users)
	m.HandleFunc("POST /v1/users", s.createUser)
	m.HandleFunc("POST /v1/users/{id}/deactivate", s.deactivateUser)
	m.HandleFunc("POST /v1/users/{id}/activate", s.activateUser)
	m.HandleFunc("DELETE /v1/users/{id}", s.deleteUser)
	m.HandleFunc("GET /v1/accounts", s.accounts)
	m.HandleFunc("POST /v1/accounts", s.createAccount)
	m.HandleFunc("PUT /v1/accounts/{id}", s.updateAccount)
	m.HandleFunc("POST /v1/accounts/{id}/deactivate", s.deactivateAccount)
	m.HandleFunc("POST /v1/accounts/{id}/activate", s.activateAccount)
	m.HandleFunc("DELETE /v1/accounts/{id}", s.deleteAccount)
	m.HandleFunc("GET /v1/categories", s.categories)
	m.HandleFunc("POST /v1/categories", s.createCategory)
	m.HandleFunc("PUT /v1/categories/{id}", s.updateCategory)
	m.HandleFunc("POST /v1/categories/{id}/deactivate", s.deactivateCategory)
	m.HandleFunc("POST /v1/categories/{id}/activate", s.activateCategory)
	m.HandleFunc("GET /v1/transactions", s.transactions)
	m.HandleFunc("POST /v1/transactions", s.createTransaction)
	m.HandleFunc("PUT /v1/transactions/{id}", s.updateTransaction)
	m.HandleFunc("POST /v1/transactions/{id}/void", s.voidTransaction)
	m.HandleFunc("GET /v1/deletion-requests", s.deletionRequests)
	m.HandleFunc("POST /v1/deletion-requests/{id}/approve", s.approveDeletionRequest)
	m.HandleFunc("POST /v1/deletion-requests/{id}/reject", s.rejectDeletionRequest)
	m.HandleFunc("POST /v1/deletion-requests/{id}/cancel", s.cancelDeletionRequest)
	m.HandleFunc("GET /v1/audit-events", s.auditEvents)
	m.HandleFunc("GET /v1/saved-filters", s.savedFilters)
	m.HandleFunc("POST /v1/saved-filters", s.createSavedFilter)
	m.HandleFunc("PUT /v1/saved-filters/{id}", s.updateSavedFilter)
	m.HandleFunc("DELETE /v1/saved-filters/{id}", s.deleteSavedFilter)
	m.HandleFunc("GET /v1/templates/categories.csv", s.categoryTemplate)
	m.HandleFunc("GET /v1/templates/transactions.csv", s.transactionTemplate)
	m.HandleFunc("POST /v1/imports/categories", s.importCategories)
	m.HandleFunc("POST /v1/imports/transactions", s.importTransactions)
	m.HandleFunc("GET /v1/exports/transactions.csv", s.exportTransactions)
	m.HandleFunc("GET /v1/exports/audit.csv", s.exportAudit)
	return s.middleware(m)
}

type correlationKey struct{}

func correlation(ctx context.Context) string { v, _ := ctx.Value(correlationKey{}).(string); return v }
func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cid := r.Header.Get("X-Correlation-ID")
		if cid == "" {
			cid = uuid.NewString()
		}
		w.Header().Set("X-Correlation-ID", cid)
		start := time.Now()
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), correlationKey{}, cid)))
		s.Log.Info("request completed", "correlation_id", cid, "component", "api", "layer", "router", "operation", r.Method+" "+r.URL.Path, "duration_ms", time.Since(start).Milliseconds())
	})
}

func jsonBody(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, e error) {
	code := http.StatusBadRequest
	if e.Error() == "forbidden" {
		code = http.StatusForbidden
	}
	write(w, code, map[string]string{"error": e.Error()})
}
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	write(w, 200, map[string]string{"status": "ok"})
}
func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	ok, e := s.App.Initialized(r.Context())
	if e != nil {
		fail(w, e)
		return
	}
	write(w, 200, map[string]any{"storage": "local_sqlite", "initialized": ok})
}
func (s *Server) initialize(w http.ResponseWriter, r *http.Request) {
	var b struct{ Email, DisplayName, Password string }
	if e := jsonBody(r, &b); e != nil {
		fail(w, e)
		return
	}
	code, e := s.App.Initialize(r.Context(), b.Email, b.DisplayName, b.Password, correlation(r.Context()))
	if e != nil {
		fail(w, e)
		return
	}
	write(w, 201, map[string]string{"recovery_code": code})
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var b struct{ Email, Password string }
	if e := jsonBody(r, &b); e != nil {
		fail(w, e)
		return
	}
	t, u, e := s.App.Login(r.Context(), b.Email, b.Password, correlation(r.Context()))
	if e != nil {
		fail(w, e)
		return
	}
	write(w, 200, map[string]any{"token": t, "user": u})
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if actor, ok := s.App.Session(token(r)); ok {
		_ = s.App.Repo.Audit(r.Context(), uuid.NewString(), actor.ID, "user_logged_out", "user", actor.ID, correlation(r.Context()), nil, nil)
	}
	s.App.Logout(token(r))
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) recover(w http.ResponseWriter, r *http.Request) {
	var b struct{ Email, RecoveryCode, NewPassword string }
	if e := jsonBody(r, &b); e != nil {
		fail(w, e)
		return
	}
	if e := s.App.Recover(r.Context(), b.Email, b.RecoveryCode, b.NewPassword, correlation(r.Context())); e != nil {
		fail(w, e)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func token(r *http.Request) string {
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
}
func (s *Server) actor(w http.ResponseWriter, r *http.Request) (domain.User, bool) {
	u, ok := s.App.Session(token(r))
	if !ok {
		write(w, 401, map[string]string{"error": "authentication required"})
	}
	return u, ok
}
func (s *Server) users(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	if e := s.App.Require(a, domain.RoleAdministrator); e != nil {
		fail(w, e)
		return
	}
	u, e := s.App.Repo.ListUsers(r.Context())
	if e != nil {
		fail(w, e)
		return
	}
	write(w, 200, u)
}
func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	var b struct{ Email, DisplayName, Password, Role string }
	if e := jsonBody(r, &b); e != nil {
		fail(w, e)
		return
	}
	if e := s.App.CreateUser(r.Context(), a, b.Email, b.DisplayName, b.Password, b.Role, correlation(r.Context())); e != nil {
		fail(w, e)
		return
	}
	w.WriteHeader(201)
}
func (s *Server) deactivateUser(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	if err := s.App.DeactivateUser(r.Context(), a, r.PathValue("id"), correlation(r.Context())); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) activateUser(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	if err := s.App.ActivateUser(r.Context(), a, r.PathValue("id"), correlation(r.Context())); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	if err := s.App.DeleteUser(r.Context(), a, r.PathValue("id"), correlation(r.Context())); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) accounts(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	includeInactive := r.URL.Query().Get("include_inactive") == "true"
	if includeInactive {
		if err := s.App.Require(a, domain.RoleAdministrator, domain.RoleManager); err != nil {
			fail(w, err)
			return
		}
	}
	var v []domain.Account
	var e error
	if includeInactive {
		v, e = s.App.Repo.ListAllAccounts(r.Context())
	} else {
		v, e = s.App.Repo.ListAccounts(r.Context())
	}
	if e != nil {
		fail(w, e)
		return
	}
	write(w, 200, v)
}
func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	var b struct{ Name, Type string }
	if e := jsonBody(r, &b); e != nil {
		fail(w, e)
		return
	}
	if e := s.App.CreateAccount(r.Context(), a, b.Name, b.Type, correlation(r.Context())); e != nil {
		fail(w, e)
		return
	}
	w.WriteHeader(201)
}
func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	var b struct{ Name, Type string }
	if e := jsonBody(r, &b); e != nil {
		fail(w, e)
		return
	}
	if e := s.App.UpdateAccount(r.Context(), a, r.PathValue("id"), b.Name, b.Type, correlation(r.Context())); e != nil {
		fail(w, e)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) deactivateAccount(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	if e := s.App.DeactivateAccount(r.Context(), a, r.PathValue("id"), correlation(r.Context())); e != nil {
		fail(w, e)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) activateAccount(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	if err := s.App.ActivateAccount(r.Context(), a, r.PathValue("id"), correlation(r.Context())); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	var body struct {
		ReplacementAccountID string `json:"replacementAccountId"`
	}
	if r.Body != nil {
		_ = jsonBody(r, &body)
	}
	if err := s.App.DeleteAccount(r.Context(), a, r.PathValue("id"), body.ReplacementAccountID, correlation(r.Context())); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) categories(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.actor(w, r); !ok {
		return
	}
	var v []domain.Category
	var e error
	if r.URL.Query().Get("include_inactive") == "true" {
		v, e = s.App.Repo.ListAllCategories(r.Context())
	} else {
		v, e = s.App.Repo.ListCategories(r.Context())
	}
	if e != nil {
		fail(w, e)
		return
	}
	write(w, 200, v)
}
func (s *Server) createCategory(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	var b struct{ Name, Direction string }
	if e := jsonBody(r, &b); e != nil {
		fail(w, e)
		return
	}
	if e := s.App.CreateCategory(r.Context(), a, b.Name, b.Direction, correlation(r.Context())); e != nil {
		fail(w, e)
		return
	}
	w.WriteHeader(201)
}
func (s *Server) updateCategory(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	var b struct{ Name, Direction string }
	if e := jsonBody(r, &b); e != nil {
		fail(w, e)
		return
	}
	if e := s.App.UpdateCategory(r.Context(), a, r.PathValue("id"), b.Name, b.Direction, correlation(r.Context())); e != nil {
		fail(w, e)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) deactivateCategory(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	var body struct {
		ReplacementCategoryID string `json:"replacementCategoryId"`
	}
	if r.Body != nil {
		_ = jsonBody(r, &body)
	}
	if e := s.App.DeactivateCategory(r.Context(), a, r.PathValue("id"), body.ReplacementCategoryID, correlation(r.Context())); e != nil {
		fail(w, e)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) activateCategory(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	if e := s.App.ActivateCategory(r.Context(), a, r.PathValue("id"), correlation(r.Context())); e != nil {
		fail(w, e)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) transactions(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	var v []domain.Transaction
	var e error
	if a.Role == string(domain.RoleOperator) {
		v, e = s.App.Repo.ListTransactionsByCreator(r.Context(), r.URL.Query().Get("from"), r.URL.Query().Get("to"), a.ID)
	} else {
		v, e = s.App.Repo.ListTransactions(r.Context(), r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	}
	if e != nil {
		fail(w, e)
		return
	}
	write(w, 200, v)
}
func (s *Server) createTransaction(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	var t domain.Transaction
	if e := jsonBody(r, &t); e != nil {
		fail(w, e)
		return
	}
	if e := s.App.CreateTransaction(r.Context(), a, t, correlation(r.Context())); e != nil {
		fail(w, e)
		return
	}
	w.WriteHeader(201)
}
func (s *Server) updateTransaction(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	var t domain.Transaction
	if e := jsonBody(r, &t); e != nil {
		fail(w, e)
		return
	}
	t.ID = r.PathValue("id")
	if e := s.App.UpdateTransaction(r.Context(), a, t, correlation(r.Context())); e != nil {
		fail(w, e)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) voidTransaction(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if r.Body != nil {
		_ = jsonBody(r, &body)
	}
	if e := s.App.VoidTransaction(r.Context(), a, r.PathValue("id"), body.Reason, correlation(r.Context())); e != nil {
		fail(w, e)
		return
	}
	if a.Role == string(domain.RoleOperator) {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) deletionRequests(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	var items []domain.DeletionRequest
	var e error
	if a.Role == string(domain.RoleOperator) {
		items, e = s.App.Repo.ListDeletionRequestsByRequester(r.Context(), a.ID)
	} else if e = s.App.Require(a, domain.RoleAdministrator, domain.RoleManager); e == nil {
		items, e = s.App.Repo.ListDeletionRequests(r.Context())
	}
	if e != nil {
		fail(w, e)
		return
	}
	write(w, http.StatusOK, items)
}
func (s *Server) approveDeletionRequest(w http.ResponseWriter, r *http.Request) {
	s.resolveDeletionRequest(w, r, "approved")
}
func (s *Server) rejectDeletionRequest(w http.ResponseWriter, r *http.Request) {
	s.resolveDeletionRequest(w, r, "rejected")
}
func (s *Server) cancelDeletionRequest(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	if err := s.App.CancelDeletionRequest(r.Context(), a, r.PathValue("id"), correlation(r.Context())); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) auditEvents(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	if err := s.App.Require(a, domain.RoleAdministrator, domain.RoleManager); err != nil {
		fail(w, err)
		return
	}
	events, err := s.App.Repo.ListAuditEvents(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, events)
}
func (s *Server) savedFilters(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	filters, err := s.App.Repo.ListSavedFilters(r.Context(), a.ID)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, filters)
}
func (s *Server) createSavedFilter(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	var body struct {
		Name  string `json:"name"`
		Query string `json:"query"`
	}
	if err := jsonBody(r, &body); err != nil {
		fail(w, err)
		return
	}
	if err := s.App.SaveFilter(r.Context(), a, body.Name, body.Query, correlation(r.Context())); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}
func (s *Server) updateSavedFilter(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	var body struct {
		Name  string `json:"name"`
		Query string `json:"query"`
	}
	if err := jsonBody(r, &body); err != nil {
		fail(w, err)
		return
	}
	if err := s.App.UpdateSavedFilter(r.Context(), a, r.PathValue("id"), body.Name, body.Query, correlation(r.Context())); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) deleteSavedFilter(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	if err := s.App.DeleteSavedFilter(r.Context(), a, r.PathValue("id"), correlation(r.Context())); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) resolveDeletionRequest(w http.ResponseWriter, r *http.Request, decision string) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	if e := s.App.ResolveDeletionRequest(r.Context(), a, r.PathValue("id"), decision, correlation(r.Context())); e != nil {
		fail(w, e)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func csvBody(r *http.Request, expected []string) ([][]string, error) {
	var body struct {
		CSV string `json:"csv"`
	}
	if err := jsonBody(r, &body); err != nil {
		return nil, err
	}
	reader := csv.NewReader(strings.NewReader(body.CSV))
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("invalid CSV: %w", err)
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("CSV must include a header and at least one data row")
	}
	if len(rows[0]) != len(expected) {
		return nil, fmt.Errorf("invalid CSV headers")
	}
	for i, header := range expected {
		if strings.TrimSpace(rows[0][i]) != header {
			return nil, fmt.Errorf("invalid CSV headers")
		}
	}
	for index, row := range rows[1:] {
		if len(row) != len(expected) {
			return nil, fmt.Errorf("row %d has %d columns; expected %d", index+2, len(row), len(expected))
		}
	}
	return rows[1:], nil
}
func template(w http.ResponseWriter, name string, headers, example []string) {
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename="+name)
	writer := csv.NewWriter(w)
	_ = writer.Write(headers)
	_ = writer.Write(example)
	writer.Flush()
}
func (s *Server) categoryTemplate(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.actor(w, r); !ok {
		return
	}
	template(w, "cashflow-categories-template.csv", []string{"name", "direction"}, []string{"Office supplies", "expense"})
}
func (s *Server) transactionTemplate(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.actor(w, r); !ok {
		return
	}
	template(w, "cashflow-movements-template.csv", []string{"account_name", "category_name", "direction", "amount_minor", "currency", "description", "occurred_on"}, []string{"Cash", "Office supplies", "expense", "125000", "ARS", "Printer supplies", "2026-08-22"})
}
func (s *Server) importCategories(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	rows, err := csvBody(r, []string{"name", "direction"})
	if err != nil {
		fail(w, err)
		return
	}
	items := make([]application.CategoryImport, 0, len(rows))
	for _, row := range rows {
		items = append(items, application.CategoryImport{Name: row[0], Direction: row[1]})
	}
	if err = s.App.ImportCategories(r.Context(), a, items, correlation(r.Context())); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}
func (s *Server) importTransactions(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	rows, err := csvBody(r, []string{"account_name", "category_name", "direction", "amount_minor", "currency", "description", "occurred_on"})
	if err != nil {
		fail(w, err)
		return
	}
	items := make([]application.TransactionImport, 0, len(rows))
	for index, row := range rows {
		amount, parseErr := strconv.ParseInt(strings.TrimSpace(row[3]), 10, 64)
		if parseErr != nil || amount <= 0 {
			fail(w, fmt.Errorf("row %d: amount_minor must be a positive integer", index+2))
			return
		}
		items = append(items, application.TransactionImport{AccountName: row[0], CategoryName: row[1], Direction: row[2], AmountMinor: amount, Currency: row[4], Description: row[5], OccurredOn: row[6]})
	}
	if err = s.App.ImportTransactions(r.Context(), a, items, correlation(r.Context())); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}
func (s *Server) exportTransactions(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	if e := s.App.Require(a, domain.RoleAdministrator, domain.RoleManager); e != nil {
		fail(w, e)
		return
	}
	v, e := s.App.Repo.ListTransactions(r.Context(), "", "")
	if e != nil {
		fail(w, e)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=transactions.csv")
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"id", "account_id", "category_id", "direction", "amount_minor", "currency", "description", "occurred_on", "status", "created_by", "created_at", "updated_at"})
	for _, t := range v {
		_ = cw.Write([]string{t.ID, t.AccountID, t.CategoryID, t.Direction, formatInt(t.AmountMinor), t.Currency, t.Description, t.OccurredOn, t.Status, t.CreatedBy, t.CreatedAt, t.UpdatedAt})
	}
	cw.Flush()
}
func (s *Server) exportAudit(w http.ResponseWriter, r *http.Request) {
	a, ok := s.actor(w, r)
	if !ok {
		return
	}
	if e := s.App.Require(a, domain.RoleAdministrator, domain.RoleManager); e != nil {
		fail(w, e)
		return
	}
	rows, e := s.App.Repo.AuditRows(r.Context())
	if e != nil {
		fail(w, e)
		return
	}
	defer rows.Close()
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=audit.csv")
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"id", "actor_id", "actor_name", "action", "entity_type", "entity_id", "correlation_id", "before", "after", "created_at"})
	for rows.Next() {
		var v [10]string
		if e = rows.Scan(&v[0], &v[1], &v[2], &v[3], &v[4], &v[5], &v[6], &v[7], &v[8]); e == nil {
			_ = cw.Write(v[:])
		}
	}
	cw.Flush()
}
func formatInt(v int64) string { return strconv.FormatInt(v, 10) }
