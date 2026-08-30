package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
)

// OpenMySQL opens a CashFlow MySQL database and creates the current schema when
// it is empty. Credentials are supplied only by the caller and are never logged.
func OpenMySQL(host, port, database, username, password string) (*Repository, error) {
	config := mysql.NewConfig()
	config.User = username
	config.Passwd = password
	config.Net = "tcp"
	config.Addr = host + ":" + port
	config.DBName = database
	config.ParseTime = true
	config.MultiStatements = true
	config.Loc = time.UTC
	config.Params = map[string]string{"charset": "utf8mb4"}
	db, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		return nil, err
	}
	if err = db.PingContext(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to MySQL: %w", err)
	}
	r := &Repository{DB: db, mysql: true}
	if err = r.migrateMySQL(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return r, nil
}

func (r *Repository) migrateMySQL(ctx context.Context) error {
	_, err := r.DB.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (version INT PRIMARY KEY, applied_at VARCHAR(64) NOT NULL);
CREATE TABLE IF NOT EXISTS users (id VARCHAR(64) PRIMARY KEY, email VARCHAR(255) NOT NULL UNIQUE, display_name VARCHAR(255) NOT NULL, password_hash TEXT NOT NULL, recovery_hash TEXT NULL, role VARCHAR(32) NOT NULL, active BOOLEAN NOT NULL DEFAULT TRUE, must_change_password BOOLEAN NOT NULL DEFAULT FALSE, created_at VARCHAR(64) NOT NULL, updated_at VARCHAR(64) NOT NULL);
CREATE TABLE IF NOT EXISTS accounts (id VARCHAR(64) PRIMARY KEY, name VARCHAR(255) NOT NULL UNIQUE, type VARCHAR(32) NOT NULL, active BOOLEAN NOT NULL DEFAULT TRUE, created_at VARCHAR(64) NOT NULL, updated_at VARCHAR(64) NOT NULL);
CREATE TABLE IF NOT EXISTS categories (id VARCHAR(64) PRIMARY KEY, name VARCHAR(255) NOT NULL UNIQUE, direction VARCHAR(32) NOT NULL, active BOOLEAN NOT NULL DEFAULT TRUE, created_at VARCHAR(64) NOT NULL, updated_at VARCHAR(64) NOT NULL);
CREATE TABLE IF NOT EXISTS transactions (id VARCHAR(64) PRIMARY KEY, account_id VARCHAR(64) NOT NULL, category_id VARCHAR(64) NULL, direction VARCHAR(32) NOT NULL, amount_minor BIGINT NOT NULL, currency VARCHAR(16) NOT NULL, description TEXT NOT NULL, occurred_on VARCHAR(32) NOT NULL, status VARCHAR(32) NOT NULL DEFAULT 'active', created_by VARCHAR(64) NOT NULL, created_at VARCHAR(64) NOT NULL, updated_at VARCHAR(64) NOT NULL, CONSTRAINT transactions_account_fk FOREIGN KEY (account_id) REFERENCES accounts(id), CONSTRAINT transactions_category_fk FOREIGN KEY (category_id) REFERENCES categories(id), CONSTRAINT transactions_user_fk FOREIGN KEY (created_by) REFERENCES users(id));
CREATE TABLE IF NOT EXISTS audit_events (id VARCHAR(64) PRIMARY KEY, actor_id VARCHAR(64) NULL, actor_name VARCHAR(255) NOT NULL DEFAULT 'System', action VARCHAR(128) NOT NULL, entity_type VARCHAR(64) NOT NULL, entity_id VARCHAR(64) NOT NULL, correlation_id VARCHAR(64) NOT NULL, before_json JSON NULL, after_json JSON NULL, created_at VARCHAR(64) NOT NULL);
CREATE TABLE IF NOT EXISTS deletion_requests (id VARCHAR(64) PRIMARY KEY, entity_type VARCHAR(64) NOT NULL, entity_id VARCHAR(64) NOT NULL, requested_by VARCHAR(64) NOT NULL, requested_by_name VARCHAR(255) NOT NULL DEFAULT '', reason TEXT NOT NULL, status VARCHAR(32) NOT NULL, resolved_by VARCHAR(64) NULL, created_at VARCHAR(64) NOT NULL, resolved_at VARCHAR(64) NULL, CONSTRAINT deletion_requests_requester_fk FOREIGN KEY (requested_by) REFERENCES users(id), CONSTRAINT deletion_requests_resolver_fk FOREIGN KEY (resolved_by) REFERENCES users(id));
CREATE TABLE IF NOT EXISTS saved_filters (id VARCHAR(64) PRIMARY KEY, user_id VARCHAR(64) NOT NULL, name VARCHAR(255) NOT NULL, query TEXT NOT NULL, created_at VARCHAR(64) NOT NULL, updated_at VARCHAR(64) NOT NULL, UNIQUE KEY saved_filters_user_name (user_id, name), CONSTRAINT saved_filters_user_fk FOREIGN KEY (user_id) REFERENCES users(id));
CREATE TABLE IF NOT EXISTS remembered_sessions (token_hash VARCHAR(128) PRIMARY KEY, user_id VARCHAR(64) NOT NULL, expires_at VARCHAR(64) NOT NULL, created_at VARCHAR(64) NOT NULL, INDEX remembered_sessions_user_id (user_id), CONSTRAINT remembered_sessions_user_fk FOREIGN KEY (user_id) REFERENCES users(id));
CREATE TABLE IF NOT EXISTS mcp_settings (id INT PRIMARY KEY, enabled BOOLEAN NOT NULL DEFAULT FALSE, exposure_mode VARCHAR(16) NOT NULL DEFAULT 'local', updated_at VARCHAR(64) NOT NULL);
CREATE TABLE IF NOT EXISTS mcp_api_keys (id VARCHAR(64) PRIMARY KEY, name VARCHAR(255) NOT NULL, user_id VARCHAR(64) NOT NULL, secret_hash TEXT NOT NULL, scopes VARCHAR(255) NOT NULL, created_at VARCHAR(64) NOT NULL, last_used_at VARCHAR(64) NULL, revoked_at VARCHAR(64) NULL, UNIQUE KEY mcp_api_keys_secret_hash (secret_hash(191)), INDEX mcp_api_keys_user_id (user_id), CONSTRAINT mcp_api_keys_user_fk FOREIGN KEY (user_id) REFERENCES users(id));
`)
	if err != nil {
		return err
	}
	if _, err = r.DB.ExecContext(ctx, "INSERT IGNORE INTO mcp_settings(id,enabled,exposure_mode,updated_at) VALUES (1,FALSE,'local',UTC_TIMESTAMP())"); err != nil {
		return err
	}
	for version := 1; version <= 9; version++ {
		if _, err = r.DB.ExecContext(ctx, "INSERT INTO schema_migrations(version, applied_at) VALUES (?, UTC_TIMESTAMP()) ON DUPLICATE KEY UPDATE version=version", version); err != nil {
			return err
		}
	}
	return nil
}
