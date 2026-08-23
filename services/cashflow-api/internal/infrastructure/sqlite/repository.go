package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cashflow/desktop/api/internal/domain"
	_ "modernc.org/sqlite"
	"time"
)

type Repository struct{ DB *sql.DB }

func Open(path string) (*Repository, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	r := &Repository{DB: db}
	if err := r.Migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return r, nil
}
func (r *Repository) Close() error { return r.DB.Close() }
func (r *Repository) Migrate(ctx context.Context) error {
	_, err := r.DB.ExecContext(ctx, `
 CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS users (id TEXT PRIMARY KEY, email TEXT NOT NULL UNIQUE, display_name TEXT NOT NULL, password_hash TEXT NOT NULL, recovery_hash TEXT, role TEXT NOT NULL CHECK(role IN ('administrator','operator')), active INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS accounts (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, type TEXT NOT NULL CHECK(type IN ('cash','bank','wallet')), active INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS categories (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, direction TEXT NOT NULL CHECK(direction IN ('income','expense','both')), active INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS transactions (id TEXT PRIMARY KEY, account_id TEXT NOT NULL REFERENCES accounts(id), category_id TEXT REFERENCES categories(id), direction TEXT NOT NULL CHECK(direction IN ('income','expense')), amount_minor INTEGER NOT NULL CHECK(amount_minor > 0), currency TEXT NOT NULL, description TEXT NOT NULL, occurred_on TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','voided')), created_by TEXT NOT NULL REFERENCES users(id), created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS audit_events (id TEXT PRIMARY KEY, actor_id TEXT, action TEXT NOT NULL, entity_type TEXT NOT NULL, entity_id TEXT NOT NULL, correlation_id TEXT NOT NULL, before_json TEXT, after_json TEXT, created_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS deletion_requests (id TEXT PRIMARY KEY, entity_type TEXT NOT NULL, entity_id TEXT NOT NULL, requested_by TEXT NOT NULL REFERENCES users(id), status TEXT NOT NULL CHECK(status IN ('pending','approved','rejected')), resolved_by TEXT REFERENCES users(id), created_at TEXT NOT NULL, resolved_at TEXT);
 INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (1, datetime('now'));
`)
	if err != nil {
		return err
	}
	var applied int
	err = r.DB.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations WHERE version=2").Scan(&applied)
	if err != nil || applied > 0 {
		return err
	}
	if _, err = r.DB.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		return err
	}
	defer r.DB.ExecContext(context.Background(), "PRAGMA foreign_keys=ON")
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `CREATE TABLE users_next (id TEXT PRIMARY KEY, email TEXT NOT NULL UNIQUE, display_name TEXT NOT NULL, password_hash TEXT NOT NULL, recovery_hash TEXT, role TEXT NOT NULL CHECK(role IN ('administrator','manager','operator')), active INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
INSERT INTO users_next SELECT id,email,display_name,password_hash,recovery_hash,role,active,created_at,updated_at FROM users;
DROP TABLE users;
ALTER TABLE users_next RENAME TO users;
INSERT INTO schema_migrations(version, applied_at) VALUES (2, datetime('now'));`)
	if err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}
func (r *Repository) Initialized(ctx context.Context) (bool, error) {
	var n int
	err := r.DB.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE role='administrator'").Scan(&n)
	return n > 0, err
}
func (r *Repository) CreateUser(ctx context.Context, id, email, name, hash, recovery, role string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.DB.ExecContext(ctx, "INSERT INTO users(id,email,display_name,password_hash,recovery_hash,role,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)", id, email, name, hash, recovery, role, now, now)
	return err
}
func (r *Repository) UserCredentials(ctx context.Context, email string) (domain.User, string, string, error) {
	var u domain.User
	var hash, recovery string
	err := r.DB.QueryRowContext(ctx, "SELECT id,email,display_name,role,active,password_hash,COALESCE(recovery_hash,'') FROM users WHERE email=? AND active=1", email).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &u.Active, &hash, &recovery)
	return u, hash, recovery, err
}
func (r *Repository) UpdatePassword(ctx context.Context, id, hash string) error {
	_, err := r.DB.ExecContext(ctx, "UPDATE users SET password_hash=?, recovery_hash=NULL, updated_at=? WHERE id=?", hash, time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}
func (r *Repository) ListUsers(ctx context.Context) ([]domain.User, error) {
	rows, e := r.DB.QueryContext(ctx, "SELECT id,email,display_name,role,active FROM users ORDER BY active DESC, email")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := make([]domain.User, 0)
	for rows.Next() {
		var u domain.User
		if e = rows.Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &u.Active); e != nil {
			return nil, e
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
func (r *Repository) User(ctx context.Context, id string) (domain.User, error) {
	var u domain.User
	err := r.DB.QueryRowContext(ctx, "SELECT id,email,display_name,role,active FROM users WHERE id=?", id).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &u.Active)
	return u, err
}
func (r *Repository) SetUserActive(ctx context.Context, id string, active bool) error {
	_, err := r.DB.ExecContext(ctx, "UPDATE users SET active=?, updated_at=? WHERE id=?", active, time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}
func (r *Repository) ActiveAdministratorCount(ctx context.Context) (int, error) {
	var count int
	err := r.DB.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE role='administrator' AND active=1").Scan(&count)
	return count, err
}
func (r *Repository) DeleteUser(ctx context.Context, id string) error {
	_, err := r.DB.ExecContext(ctx, "DELETE FROM users WHERE id=?", id)
	return err
}
func (r *Repository) CreateAccount(ctx context.Context, a domain.Account) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, e := r.DB.ExecContext(ctx, "INSERT INTO accounts(id,name,type,created_at,updated_at) VALUES(?,?,?,?,?)", a.ID, a.Name, a.Type, now, now)
	return e
}
func (r *Repository) ListAccounts(ctx context.Context) ([]domain.Account, error) {
	rows, e := r.DB.QueryContext(ctx, "SELECT id,name,type,active FROM accounts WHERE active=1 ORDER BY name")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	o := make([]domain.Account, 0)
	for rows.Next() {
		var a domain.Account
		var x int
		if e = rows.Scan(&a.ID, &a.Name, &a.Type, &x); e != nil {
			return nil, e
		}
		a.Active = x == 1
		o = append(o, a)
	}
	return o, rows.Err()
}
func (r *Repository) Account(ctx context.Context, id string) (domain.Account, error) {
	var a domain.Account
	var active int
	err := r.DB.QueryRowContext(ctx, "SELECT id,name,type,active FROM accounts WHERE id=?", id).Scan(&a.ID, &a.Name, &a.Type, &active)
	a.Active = active == 1
	return a, err
}
func (r *Repository) AccountByName(ctx context.Context, name string) (domain.Account, error) {
	var a domain.Account
	var active int
	err := r.DB.QueryRowContext(ctx, "SELECT id,name,type,active FROM accounts WHERE name=? AND active=1", name).Scan(&a.ID, &a.Name, &a.Type, &active)
	a.Active = active == 1
	return a, err
}
func (r *Repository) UpdateAccount(ctx context.Context, a domain.Account) error {
	res, e := r.DB.ExecContext(ctx, "UPDATE accounts SET name=?,type=?,updated_at=? WHERE id=? AND active=1", a.Name, a.Type, time.Now().UTC().Format(time.RFC3339Nano), a.ID)
	if e != nil {
		return e
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("account is inactive or does not exist")
	}
	return nil
}
func (r *Repository) DeactivateAccount(ctx context.Context, id string) error {
	res, e := r.DB.ExecContext(ctx, "UPDATE accounts SET active=0,updated_at=? WHERE id=? AND active=1", time.Now().UTC().Format(time.RFC3339Nano), id)
	if e != nil {
		return e
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("account is inactive or does not exist")
	}
	return nil
}
func (r *Repository) CreateCategory(ctx context.Context, c domain.Category) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, e := r.DB.ExecContext(ctx, "INSERT INTO categories(id,name,direction,created_at,updated_at) VALUES(?,?,?,?,?)", c.ID, c.Name, c.Direction, now, now)
	return e
}
func (r *Repository) ListCategories(ctx context.Context) ([]domain.Category, error) {
	rows, e := r.DB.QueryContext(ctx, "SELECT id,name,direction,active FROM categories WHERE active=1 ORDER BY name")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	o := make([]domain.Category, 0)
	for rows.Next() {
		var c domain.Category
		var x int
		if e = rows.Scan(&c.ID, &c.Name, &c.Direction, &x); e != nil {
			return nil, e
		}
		c.Active = x == 1
		o = append(o, c)
	}
	return o, rows.Err()
}
func (r *Repository) Category(ctx context.Context, id string) (domain.Category, error) {
	var c domain.Category
	var active int
	err := r.DB.QueryRowContext(ctx, "SELECT id,name,direction,active FROM categories WHERE id=?", id).Scan(&c.ID, &c.Name, &c.Direction, &active)
	c.Active = active == 1
	return c, err
}
func (r *Repository) CategoryByName(ctx context.Context, name string) (domain.Category, error) {
	var c domain.Category
	var active int
	err := r.DB.QueryRowContext(ctx, "SELECT id,name,direction,active FROM categories WHERE name=? AND active=1", name).Scan(&c.ID, &c.Name, &c.Direction, &active)
	c.Active = active == 1
	return c, err
}
func (r *Repository) UpdateCategory(ctx context.Context, c domain.Category) error {
	res, e := r.DB.ExecContext(ctx, "UPDATE categories SET name=?,direction=?,updated_at=? WHERE id=? AND active=1", c.Name, c.Direction, time.Now().UTC().Format(time.RFC3339Nano), c.ID)
	if e != nil {
		return e
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("category is inactive or does not exist")
	}
	return nil
}
func (r *Repository) DeactivateCategory(ctx context.Context, id string) error {
	res, e := r.DB.ExecContext(ctx, "UPDATE categories SET active=0,updated_at=? WHERE id=? AND active=1", time.Now().UTC().Format(time.RFC3339Nano), id)
	if e != nil {
		return e
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("category is inactive or does not exist")
	}
	return nil
}
func (r *Repository) CreateTransaction(ctx context.Context, t domain.Transaction) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, e := r.DB.ExecContext(ctx, "INSERT INTO transactions(id,account_id,category_id,direction,amount_minor,currency,description,occurred_on,status,created_by,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)", t.ID, t.AccountID, null(t.CategoryID), t.Direction, t.AmountMinor, t.Currency, t.Description, t.OccurredOn, "active", t.CreatedBy, now, now)
	return e
}
func (r *Repository) ListTransactions(ctx context.Context, from, to string) ([]domain.Transaction, error) {
	return r.listTransactions(ctx, from, to, "")
}
func (r *Repository) ListTransactionsByCreator(ctx context.Context, from, to, creator string) ([]domain.Transaction, error) {
	return r.listTransactions(ctx, from, to, creator)
}
func (r *Repository) listTransactions(ctx context.Context, from, to, creator string) ([]domain.Transaction, error) {
	q := "SELECT id,account_id,COALESCE(category_id,''),direction,amount_minor,currency,description,occurred_on,status,created_by,created_at,updated_at FROM transactions WHERE 1=1"
	args := []any{}
	if from != "" {
		q += " AND occurred_on>=?"
		args = append(args, from)
	}
	if to != "" {
		q += " AND occurred_on<=?"
		args = append(args, to)
	}
	if creator != "" {
		q += " AND created_by=?"
		args = append(args, creator)
	}
	q += " ORDER BY occurred_on DESC,created_at DESC"
	rows, e := r.DB.QueryContext(ctx, q, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	o := make([]domain.Transaction, 0)
	for rows.Next() {
		var t domain.Transaction
		if e = rows.Scan(&t.ID, &t.AccountID, &t.CategoryID, &t.Direction, &t.AmountMinor, &t.Currency, &t.Description, &t.OccurredOn, &t.Status, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); e != nil {
			return nil, e
		}
		o = append(o, t)
	}
	return o, rows.Err()
}
func (r *Repository) VoidTransaction(ctx context.Context, id string) error {
	res, e := r.DB.ExecContext(ctx, "UPDATE transactions SET status='voided',updated_at=? WHERE id=? AND status='active'", time.Now().UTC().Format(time.RFC3339Nano), id)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("transaction is not active or does not exist")
	}
	return nil
}
func (r *Repository) UpdateTransaction(ctx context.Context, t domain.Transaction) error {
	res, e := r.DB.ExecContext(ctx, "UPDATE transactions SET account_id=?,category_id=?,direction=?,amount_minor=?,currency=?,description=?,occurred_on=?,updated_at=? WHERE id=? AND status='active'", t.AccountID, null(t.CategoryID), t.Direction, t.AmountMinor, t.Currency, t.Description, t.OccurredOn, time.Now().UTC().Format(time.RFC3339Nano), t.ID)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("transaction is not active or does not exist")
	}
	return nil
}
func (r *Repository) CreateDeletionRequest(ctx context.Context, id, entityType, entityID, requestedBy string) error {
	_, err := r.DB.ExecContext(ctx, "INSERT INTO deletion_requests(id,entity_type,entity_id,requested_by,status,created_at) VALUES(?,?,?,?,?,?)", id, entityType, entityID, requestedBy, "pending", time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
func (r *Repository) DeletionRequest(ctx context.Context, id string) (domain.DeletionRequest, error) {
	var request domain.DeletionRequest
	err := r.DB.QueryRowContext(ctx, "SELECT id,entity_type,entity_id,requested_by,status,created_at FROM deletion_requests WHERE id=?", id).Scan(&request.ID, &request.EntityType, &request.EntityID, &request.RequestedBy, &request.Status, &request.CreatedAt)
	return request, err
}
func (r *Repository) ListDeletionRequests(ctx context.Context) ([]domain.DeletionRequest, error) {
	rows, err := r.DB.QueryContext(ctx, "SELECT id,entity_type,entity_id,requested_by,status,created_at FROM deletion_requests ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.DeletionRequest, 0)
	for rows.Next() {
		var request domain.DeletionRequest
		if err = rows.Scan(&request.ID, &request.EntityType, &request.EntityID, &request.RequestedBy, &request.Status, &request.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, request)
	}
	return items, rows.Err()
}
func (r *Repository) ResolveDeletionRequest(ctx context.Context, id, status, resolver string) error {
	result, err := r.DB.ExecContext(ctx, "UPDATE deletion_requests SET status=?,resolved_by=?,resolved_at=? WHERE id=? AND status='pending'", status, resolver, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return errors.New("deletion request is not pending or does not exist")
	}
	return nil
}
func (r *Repository) Audit(ctx context.Context, id, actor, action, entityType, entityID, correlation string, before, after any) error {
	b, _ := json.Marshal(before)
	a, _ := json.Marshal(after)
	_, e := r.DB.ExecContext(ctx, "INSERT INTO audit_events(id,actor_id,action,entity_type,entity_id,correlation_id,before_json,after_json,created_at) VALUES(?,?,?,?,?,?,?,?,?)", id, null(actor), action, entityType, entityID, correlation, string(b), string(a), time.Now().UTC().Format(time.RFC3339Nano))
	return e
}
func (r *Repository) AuditRows(ctx context.Context) (*sql.Rows, error) {
	return r.DB.QueryContext(ctx, "SELECT id,COALESCE(actor_id,''),action,entity_type,entity_id,correlation_id,before_json,after_json,created_at FROM audit_events ORDER BY created_at DESC")
}
func null(v string) any {
	if v == "" {
		return nil
	}
	return v
}
func (r *Repository) EnsureDefaultCategories(ctx context.Context) error {
	for _, c := range []domain.Category{{"seed-income", "General income", "income", true}, {"seed-expense", "General expense", "expense", true}} {
		_, e := r.DB.ExecContext(ctx, "INSERT OR IGNORE INTO categories(id,name,direction,active,created_at,updated_at) VALUES(?,?,?,?,?,?)", c.ID, c.Name, c.Direction, 1, time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
		if e != nil {
			return fmt.Errorf("seed category: %w", e)
		}
	}
	return nil
}
