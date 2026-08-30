package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cashflow/desktop/api/internal/domain"
	_ "modernc.org/sqlite"
	"os"
	"time"
)

type Repository struct {
	DB    *sql.DB
	mysql bool
}

// Validate checks that an existing SQLite file is readable and is either empty
// or a database created by a compatible CashFlow version. It never migrates or
// changes the selected file.
func Validate(path string) error {
	header := make([]byte, 16)
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err = file.Read(header); err != nil || string(header) != "SQLite format 3\x00" {
		return errors.New("the selected file is not a valid SQLite database")
	}
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return err
	}
	defer db.Close()
	var integrity string
	if err = db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return fmt.Errorf("the selected SQLite database is corrupt: %s", integrity)
	}
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type = 'table'")
	if err != nil {
		return err
	}
	defer rows.Close()
	tables := map[string]bool{}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return err
		}
		tables[name] = true
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if len(tables) == 0 {
		return nil
	}
	for _, required := range []string{"schema_migrations", "users", "accounts", "categories", "transactions", "audit_events", "deletion_requests"} {
		if !tables[required] {
			return errors.New("the selected SQLite database does not match the CashFlow schema")
		}
	}
	return nil
}

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
func (r *Repository) Close() error                   { return r.DB.Close() }
func (r *Repository) Ping(ctx context.Context) error { return r.DB.PingContext(ctx) }
func (r *Repository) IsMySQL() bool                  { return r.mysql }
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
	if err != nil {
		return err
	}
	if applied == 0 {
		if _, err = r.DB.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
			return err
		}
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
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	if _, err = r.DB.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		return err
	}
	err = r.DB.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations WHERE version=3").Scan(&applied)
	if err != nil {
		return err
	}
	if applied == 0 {
		if _, err = r.DB.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
			return err
		}
		tx, err := r.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `CREATE TABLE deletion_requests_next (id TEXT PRIMARY KEY, entity_type TEXT NOT NULL, entity_id TEXT NOT NULL, requested_by TEXT NOT NULL REFERENCES users(id), reason TEXT NOT NULL, status TEXT NOT NULL CHECK(status IN ('pending','approved','rejected','cancelled')), resolved_by TEXT REFERENCES users(id), created_at TEXT NOT NULL, resolved_at TEXT);
INSERT INTO deletion_requests_next(id,entity_type,entity_id,requested_by,reason,status,resolved_by,created_at,resolved_at) SELECT id,entity_type,entity_id,requested_by,'',status,resolved_by,created_at,resolved_at FROM deletion_requests;
DROP TABLE deletion_requests;
ALTER TABLE deletion_requests_next RENAME TO deletion_requests;
UPDATE deletion_requests SET status='cancelled', resolved_at=datetime('now') WHERE id IN (SELECT id FROM (SELECT id,ROW_NUMBER() OVER (PARTITION BY entity_type,entity_id ORDER BY created_at DESC) AS position FROM deletion_requests WHERE status='pending') WHERE position>1);
CREATE UNIQUE INDEX deletion_requests_one_pending ON deletion_requests(entity_type,entity_id) WHERE status='pending';
INSERT INTO schema_migrations(version, applied_at) VALUES (3, datetime('now'));`)
		if err != nil {
			tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	if _, err = r.DB.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		return err
	}
	err = r.DB.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations WHERE version=4").Scan(&applied)
	if err != nil {
		return err
	}
	if applied == 0 {
		_, err = r.DB.ExecContext(ctx, `ALTER TABLE deletion_requests ADD COLUMN requested_by_name TEXT NOT NULL DEFAULT '';
UPDATE deletion_requests SET requested_by_name=COALESCE((SELECT display_name FROM users WHERE users.id=deletion_requests.requested_by),'Deleted user');
INSERT INTO schema_migrations(version, applied_at) VALUES (4, datetime('now'));`)
	}
	if err != nil {
		return err
	}
	err = r.DB.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations WHERE version=5").Scan(&applied)
	if err != nil {
		return err
	}
	if applied == 0 {
		_, err = r.DB.ExecContext(ctx, `ALTER TABLE audit_events ADD COLUMN actor_name TEXT NOT NULL DEFAULT 'System';
UPDATE audit_events SET actor_name=COALESCE((SELECT display_name FROM users WHERE users.id=audit_events.actor_id),'Deleted user') WHERE actor_id IS NOT NULL;
INSERT INTO schema_migrations(version, applied_at) VALUES (5, datetime('now'));`)
	}
	if err != nil {
		return err
	}
	err = r.DB.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations WHERE version=6").Scan(&applied)
	if err != nil {
		return err
	}
	if applied == 0 {
		_, err = r.DB.ExecContext(ctx, `CREATE TABLE saved_filters (id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id), name TEXT NOT NULL, query TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, UNIQUE(user_id,name));
INSERT INTO schema_migrations(version, applied_at) VALUES (6, datetime('now'));`)
	}
	if err != nil {
		return err
	}
	err = r.DB.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations WHERE version=7").Scan(&applied)
	if err != nil {
		return err
	}
	if applied == 0 {
		_, err = r.DB.ExecContext(ctx, `ALTER TABLE users ADD COLUMN must_change_password INTEGER NOT NULL DEFAULT 0;
INSERT INTO schema_migrations(version, applied_at) VALUES (7, datetime('now'));`)
	}
	if err != nil {
		return err
	}
	err = r.DB.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations WHERE version=8").Scan(&applied)
	if err != nil {
		return err
	}
	if applied == 0 {
		_, err = r.DB.ExecContext(ctx, `CREATE TABLE remembered_sessions (token_hash TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id), expires_at TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE INDEX remembered_sessions_user_id ON remembered_sessions(user_id);
INSERT INTO schema_migrations(version, applied_at) VALUES (8, datetime('now'));`)
	}
	if err != nil {
		return err
	}
	err = r.DB.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations WHERE version=9").Scan(&applied)
	if err != nil {
		return err
	}
	if applied == 0 {
		_, err = r.DB.ExecContext(ctx, `CREATE TABLE mcp_settings (id INTEGER PRIMARY KEY CHECK(id=1), enabled INTEGER NOT NULL DEFAULT 0, exposure_mode TEXT NOT NULL DEFAULT 'local' CHECK(exposure_mode IN ('local','remote')), updated_at TEXT NOT NULL);
CREATE TABLE mcp_api_keys (id TEXT PRIMARY KEY, name TEXT NOT NULL, user_id TEXT NOT NULL REFERENCES users(id), secret_hash TEXT NOT NULL UNIQUE, scopes TEXT NOT NULL, created_at TEXT NOT NULL, last_used_at TEXT, revoked_at TEXT);
CREATE INDEX mcp_api_keys_user_id ON mcp_api_keys(user_id);
INSERT INTO mcp_settings(id,enabled,exposure_mode,updated_at) VALUES(1,0,'local',datetime('now'));
INSERT INTO schema_migrations(version, applied_at) VALUES (9, datetime('now'));`)
	}
	return err
}
func (r *Repository) Initialized(ctx context.Context) (bool, error) {
	var n int
	err := r.DB.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE role='administrator'").Scan(&n)
	return n > 0, err
}
func (r *Repository) MCPSettings(ctx context.Context) (domain.MCPSettings, error) {
	var settings domain.MCPSettings
	err := r.DB.QueryRowContext(ctx, "SELECT enabled,exposure_mode FROM mcp_settings WHERE id=1").Scan(&settings.Enabled, &settings.ExposureMode)
	return settings, err
}
func (r *Repository) SaveMCPSettings(ctx context.Context, settings domain.MCPSettings) error {
	_, err := r.DB.ExecContext(ctx, "UPDATE mcp_settings SET enabled=?,exposure_mode=?,updated_at=? WHERE id=1", settings.Enabled, settings.ExposureMode, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
func (r *Repository) CreateMCPAPIKey(ctx context.Context, key domain.MCPAPIKey, secretHash string) error {
	_, err := r.DB.ExecContext(ctx, "INSERT INTO mcp_api_keys(id,name,user_id,secret_hash,scopes,created_at) VALUES(?,?,?,?,?,?)", key.ID, key.Name, key.UserID, secretHash, key.Scopes, key.CreatedAt)
	return err
}
func (r *Repository) ListMCPAPIKeys(ctx context.Context, userID string) ([]domain.MCPAPIKey, error) {
	rows, err := r.DB.QueryContext(ctx, "SELECT id,name,user_id,scopes,created_at,COALESCE(last_used_at,''),COALESCE(revoked_at,'') FROM mcp_api_keys WHERE user_id=? ORDER BY created_at DESC", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []domain.MCPAPIKey{}
	for rows.Next() {
		var key domain.MCPAPIKey
		if err = rows.Scan(&key.ID, &key.Name, &key.UserID, &key.Scopes, &key.CreatedAt, &key.LastUsedAt, &key.RevokedAt); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}
func (r *Repository) RevokeMCPAPIKey(ctx context.Context, id, userID string) error {
	result, err := r.DB.ExecContext(ctx, "UPDATE mcp_api_keys SET revoked_at=? WHERE id=? AND user_id=? AND revoked_at IS NULL", time.Now().UTC().Format(time.RFC3339Nano), id, userID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return errors.New("MCP API key not found or already revoked")
	}
	return nil
}
func (r *Repository) CreateUser(ctx context.Context, id, email, name, hash, recovery, role string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.DB.ExecContext(ctx, "INSERT INTO users(id,email,display_name,password_hash,recovery_hash,role,must_change_password,created_at,updated_at) VALUES(?,?,?,?,?,?,0,?,?)", id, email, name, hash, recovery, role, now, now)
	return err
}
func (r *Repository) UserCredentials(ctx context.Context, email string) (domain.User, string, string, error) {
	var u domain.User
	var hash, recovery string
	err := r.DB.QueryRowContext(ctx, "SELECT id,email,display_name,role,active,must_change_password,password_hash,COALESCE(recovery_hash,'') FROM users WHERE email=? AND active=1", email).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &u.Active, &u.MustChangePassword, &hash, &recovery)
	return u, hash, recovery, err
}
func (r *Repository) UpdatePassword(ctx context.Context, id, hash string, mustChange bool) error {
	_, err := r.DB.ExecContext(ctx, "UPDATE users SET password_hash=?, recovery_hash=NULL, must_change_password=?, updated_at=? WHERE id=?", hash, mustChange, time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}
func (r *Repository) UpdatePasswordWithRecovery(ctx context.Context, id, passwordHash, recoveryHash string) error {
	_, err := r.DB.ExecContext(ctx, "UPDATE users SET password_hash=?, recovery_hash=?, must_change_password=0, updated_at=? WHERE id=?", passwordHash, recoveryHash, time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}
func (r *Repository) UpdateRecoveryCode(ctx context.Context, id, recoveryHash string) error {
	_, err := r.DB.ExecContext(ctx, "UPDATE users SET recovery_hash=?, updated_at=? WHERE id=?", recoveryHash, time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}
func (r *Repository) ListUsers(ctx context.Context) ([]domain.User, error) {
	rows, e := r.DB.QueryContext(ctx, `SELECT u.id,u.email,u.display_name,u.role,u.active,u.must_change_password,
		NOT EXISTS (SELECT 1 FROM transactions t WHERE t.created_by=u.id)
		AND NOT EXISTS (SELECT 1 FROM saved_filters f WHERE f.user_id=u.id)
		FROM users u ORDER BY u.active DESC, u.email`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := make([]domain.User, 0)
	for rows.Next() {
		var u domain.User
		if e = rows.Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &u.Active, &u.MustChangePassword, &u.CanDelete); e != nil {
			return nil, e
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
func (r *Repository) User(ctx context.Context, id string) (domain.User, error) {
	var u domain.User
	err := r.DB.QueryRowContext(ctx, "SELECT id,email,display_name,role,active,must_change_password FROM users WHERE id=?", id).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &u.Active, &u.MustChangePassword)
	return u, err
}
func (r *Repository) CreateRememberedSession(ctx context.Context, tokenHash, userID string, expiresAt time.Time) error {
	_, err := r.DB.ExecContext(ctx, "INSERT INTO remembered_sessions(token_hash,user_id,expires_at,created_at) VALUES(?,?,?,?)", tokenHash, userID, expiresAt.UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
func (r *Repository) RememberedSession(ctx context.Context, tokenHash string) (domain.User, bool) {
	var u domain.User
	var expiresAt string
	err := r.DB.QueryRowContext(ctx, `SELECT u.id,u.email,u.display_name,u.role,u.active,u.must_change_password,s.expires_at
		FROM remembered_sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=?`, tokenHash).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &u.Active, &u.MustChangePassword, &expiresAt)
	if err != nil || !u.Active {
		return domain.User{}, false
	}
	expires, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil || !expires.After(time.Now().UTC()) {
		_, _ = r.DB.ExecContext(ctx, "DELETE FROM remembered_sessions WHERE token_hash=?", tokenHash)
		return domain.User{}, false
	}
	return u, true
}
func (r *Repository) DeleteRememberedSession(ctx context.Context, tokenHash string) {
	_, _ = r.DB.ExecContext(ctx, "DELETE FROM remembered_sessions WHERE token_hash=?", tokenHash)
}
func (r *Repository) DeleteRememberedSessionsForUser(ctx context.Context, userID string) {
	_, _ = r.DB.ExecContext(ctx, "DELETE FROM remembered_sessions WHERE user_id=?", userID)
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
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM remembered_sessions WHERE user_id=?", id); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM users WHERE id=?", id); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
func (r *Repository) CreateAccount(ctx context.Context, a domain.Account) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, e := r.DB.ExecContext(ctx, "INSERT INTO accounts(id,name,type,created_at,updated_at) VALUES(?,?,?,?,?)", a.ID, a.Name, a.Type, now, now)
	return e
}
func (r *Repository) ListAccounts(ctx context.Context) ([]domain.Account, error) {
	return r.listAccounts(ctx, false)
}
func (r *Repository) ListAllAccounts(ctx context.Context) ([]domain.Account, error) {
	return r.listAccounts(ctx, true)
}
func (r *Repository) listAccounts(ctx context.Context, includeInactive bool) ([]domain.Account, error) {
	query := "SELECT id,name,type,active FROM accounts"
	if !includeInactive {
		query += " WHERE active=1"
	}
	query += " ORDER BY active DESC,name"
	rows, e := r.DB.QueryContext(ctx, query)
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
func (r *Repository) ActivateAccount(ctx context.Context, id string) error {
	_, err := r.DB.ExecContext(ctx, "UPDATE accounts SET active=1,updated_at=? WHERE id=? AND active=0", time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}
func (r *Repository) DeleteAccount(ctx context.Context, id string) error {
	_, err := r.DB.ExecContext(ctx, "DELETE FROM accounts WHERE id=? AND active=0", id)
	return err
}
func (r *Repository) AccountUsageCount(ctx context.Context, id string) (int, error) {
	var count int
	err := r.DB.QueryRowContext(ctx, "SELECT count(*) FROM transactions WHERE account_id=?", id).Scan(&count)
	return count, err
}
func (r *Repository) MigrateAccountTransactions(ctx context.Context, fromID, toID string) error {
	_, err := r.DB.ExecContext(ctx, "UPDATE transactions SET account_id=?,updated_at=? WHERE account_id=?", toID, time.Now().UTC().Format(time.RFC3339Nano), fromID)
	return err
}
func (r *Repository) CreateCategory(ctx context.Context, c domain.Category) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, e := r.DB.ExecContext(ctx, "INSERT INTO categories(id,name,direction,created_at,updated_at) VALUES(?,?,?,?,?)", c.ID, c.Name, c.Direction, now, now)
	return e
}
func (r *Repository) ListCategories(ctx context.Context) ([]domain.Category, error) {
	return r.listCategories(ctx, false)
}
func (r *Repository) ListAllCategories(ctx context.Context) ([]domain.Category, error) {
	return r.listCategories(ctx, true)
}
func (r *Repository) listCategories(ctx context.Context, includeInactive bool) ([]domain.Category, error) {
	query := `SELECT c.id,c.name,c.direction,c.active,COUNT(t.id)
		FROM categories c LEFT JOIN transactions t ON t.category_id=c.id
		WHERE (? OR c.active=1) GROUP BY c.id,c.name,c.direction,c.active ORDER BY c.active DESC,c.name`
	rows, e := r.DB.QueryContext(ctx, query, includeInactive)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	o := make([]domain.Category, 0)
	for rows.Next() {
		var c domain.Category
		var x int
		if e = rows.Scan(&c.ID, &c.Name, &c.Direction, &x, &c.UsageCount); e != nil {
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
func (r *Repository) DeleteCategory(ctx context.Context, id string) error {
	res, err := r.DB.ExecContext(ctx, "DELETE FROM categories WHERE id=? AND active=0", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("category is active or does not exist")
	}
	return nil
}
func (r *Repository) ActivateCategory(ctx context.Context, id string) error {
	res, e := r.DB.ExecContext(ctx, "UPDATE categories SET active=1,updated_at=? WHERE id=? AND active=0", time.Now().UTC().Format(time.RFC3339Nano), id)
	if e != nil {
		return e
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("category is active or does not exist")
	}
	return nil
}
func (r *Repository) CategoryUsageCount(ctx context.Context, id string) (int, error) {
	var count int
	err := r.DB.QueryRowContext(ctx, "SELECT count(*) FROM transactions WHERE category_id=?", id).Scan(&count)
	return count, err
}
func (r *Repository) MigrateCategoryTransactions(ctx context.Context, fromID, toID string) error {
	_, err := r.DB.ExecContext(ctx, "UPDATE transactions SET category_id=?,updated_at=? WHERE category_id=?", toID, time.Now().UTC().Format(time.RFC3339Nano), fromID)
	return err
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
	q := "SELECT t.id,t.account_id,COALESCE(a.name,''),COALESCE(t.category_id,''),COALESCE(c.name,''),t.direction,t.amount_minor,t.currency,t.description,t.occurred_on,t.status,t.created_by,t.created_at,t.updated_at FROM transactions t LEFT JOIN accounts a ON a.id=t.account_id LEFT JOIN categories c ON c.id=t.category_id WHERE 1=1"
	args := []any{}
	if from != "" {
		q += " AND t.occurred_on>=?"
		args = append(args, from)
	}
	if to != "" {
		q += " AND t.occurred_on<=?"
		args = append(args, to)
	}
	if creator != "" {
		q += " AND t.created_by=?"
		args = append(args, creator)
	}
	q += " ORDER BY t.occurred_on DESC,t.created_at DESC"
	rows, e := r.DB.QueryContext(ctx, q, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	o := make([]domain.Transaction, 0)
	for rows.Next() {
		var t domain.Transaction
		if e = rows.Scan(&t.ID, &t.AccountID, &t.AccountName, &t.CategoryID, &t.CategoryName, &t.Direction, &t.AmountMinor, &t.Currency, &t.Description, &t.OccurredOn, &t.Status, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); e != nil {
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
func (r *Repository) CreateDeletionRequest(ctx context.Context, id, entityType, entityID, requestedBy, requestedByName, reason string) error {
	_, err := r.DB.ExecContext(ctx, "INSERT INTO deletion_requests(id,entity_type,entity_id,requested_by,requested_by_name,reason,status,created_at) VALUES(?,?,?,?,?,?,?,?)", id, entityType, entityID, requestedBy, requestedByName, reason, "pending", time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
func (r *Repository) DeletionRequest(ctx context.Context, id string) (domain.DeletionRequest, error) {
	var request domain.DeletionRequest
	err := r.DB.QueryRowContext(ctx, "SELECT id,entity_type,entity_id,requested_by,requested_by_name,reason,status,COALESCE(resolved_by,''),created_at,COALESCE(resolved_at,'') FROM deletion_requests WHERE id=?", id).Scan(&request.ID, &request.EntityType, &request.EntityID, &request.RequestedBy, &request.RequestedByName, &request.Reason, &request.Status, &request.ResolvedBy, &request.CreatedAt, &request.ResolvedAt)
	return request, err
}
func (r *Repository) ListDeletionRequests(ctx context.Context) ([]domain.DeletionRequest, error) {
	return r.listDeletionRequests(ctx, "")
}
func (r *Repository) ListDeletionRequestsByRequester(ctx context.Context, requester string) ([]domain.DeletionRequest, error) {
	return r.listDeletionRequests(ctx, requester)
}
func (r *Repository) listDeletionRequests(ctx context.Context, requester string) ([]domain.DeletionRequest, error) {
	query := "SELECT id,entity_type,entity_id,requested_by,requested_by_name,reason,status,COALESCE(resolved_by,''),created_at,COALESCE(resolved_at,'') FROM deletion_requests"
	args := []any{}
	if requester != "" {
		query += " WHERE requested_by=?"
		args = append(args, requester)
	}
	query += " ORDER BY created_at DESC"
	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.DeletionRequest, 0)
	for rows.Next() {
		var request domain.DeletionRequest
		if err = rows.Scan(&request.ID, &request.EntityType, &request.EntityID, &request.RequestedBy, &request.RequestedByName, &request.Reason, &request.Status, &request.ResolvedBy, &request.CreatedAt, &request.ResolvedAt); err != nil {
			return nil, err
		}
		items = append(items, request)
	}
	return items, rows.Err()
}
func (r *Repository) CancelDeletionRequest(ctx context.Context, id string) error {
	result, err := r.DB.ExecContext(ctx, "UPDATE deletion_requests SET status='cancelled',resolved_at=? WHERE id=? AND status='pending'", time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return errors.New("deletion request is not pending or does not exist")
	}
	return nil
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
	_, e := r.DB.ExecContext(ctx, `INSERT INTO audit_events(id,actor_id,actor_name,action,entity_type,entity_id,correlation_id,before_json,after_json,created_at)
		VALUES(?,?,COALESCE((SELECT display_name FROM users WHERE id=?),'System'),?,?,?,?,?,?,?)`, id, null(actor), actor, action, entityType, entityID, correlation, string(b), string(a), time.Now().UTC().Format(time.RFC3339Nano))
	return e
}
func (r *Repository) AuditRows(ctx context.Context) (*sql.Rows, error) {
	return r.DB.QueryContext(ctx, "SELECT id,COALESCE(actor_id,''),actor_name,action,entity_type,entity_id,correlation_id,before_json,after_json,created_at FROM audit_events ORDER BY created_at DESC")
}
func (r *Repository) ListAuditEvents(ctx context.Context) ([]domain.AuditEvent, error) {
	rows, err := r.DB.QueryContext(ctx, "SELECT a.id,COALESCE(a.actor_id,''),a.actor_name,a.action,a.entity_type,a.entity_id,a.correlation_id,COALESCE(a.before_json,''),COALESCE(a.after_json,''),a.created_at FROM audit_events a ORDER BY a.created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]domain.AuditEvent, 0)
	for rows.Next() {
		var event domain.AuditEvent
		if err = rows.Scan(&event.ID, &event.ActorID, &event.ActorName, &event.Action, &event.EntityType, &event.EntityID, &event.CorrelationID, &event.Before, &event.After, &event.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}
func (r *Repository) ListSavedFilters(ctx context.Context, userID string) ([]domain.SavedFilter, error) {
	rows, err := r.DB.QueryContext(ctx, "SELECT id,name,query,created_at,updated_at FROM saved_filters WHERE user_id=? ORDER BY name", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	filters := make([]domain.SavedFilter, 0)
	for rows.Next() {
		var filter domain.SavedFilter
		if err = rows.Scan(&filter.ID, &filter.Name, &filter.Query, &filter.CreatedAt, &filter.UpdatedAt); err != nil {
			return nil, err
		}
		filters = append(filters, filter)
	}
	return filters, rows.Err()
}
func (r *Repository) CreateSavedFilter(ctx context.Context, userID string, filter domain.SavedFilter) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.DB.ExecContext(ctx, "INSERT INTO saved_filters(id,user_id,name,query,created_at,updated_at) VALUES(?,?,?,?,?,?)", filter.ID, userID, filter.Name, filter.Query, now, now)
	return err
}
func (r *Repository) UpdateSavedFilter(ctx context.Context, userID, id, name, query string) error {
	result, err := r.DB.ExecContext(ctx, "UPDATE saved_filters SET name=?,query=?,updated_at=? WHERE id=? AND user_id=?", name, query, time.Now().UTC().Format(time.RFC3339Nano), id, userID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return errors.New("saved filter does not exist")
	}
	return nil
}
func (r *Repository) DeleteSavedFilter(ctx context.Context, userID, id string) error {
	result, err := r.DB.ExecContext(ctx, "DELETE FROM saved_filters WHERE id=? AND user_id=?", id, userID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return errors.New("saved filter does not exist")
	}
	return nil
}
func null(v string) any {
	if v == "" {
		return nil
	}
	return v
}
func (r *Repository) EnsureDefaultCategories(ctx context.Context) error {
	for _, c := range []domain.Category{{ID: "seed-income", Name: "General income", Direction: "income", Active: true}, {ID: "seed-expense", Name: "General expense", Direction: "expense", Active: true}} {
		statement := "INSERT OR IGNORE INTO categories(id,name,direction,active,created_at,updated_at) VALUES(?,?,?,?,?,?)"
		if r.mysql {
			statement = "INSERT IGNORE INTO categories(id,name,direction,active,created_at,updated_at) VALUES(?,?,?,?,?,?)"
		}
		_, e := r.DB.ExecContext(ctx, statement, c.ID, c.Name, c.Direction, 1, time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
		if e != nil {
			return fmt.Errorf("seed category: %w", e)
		}
	}
	return nil
}
