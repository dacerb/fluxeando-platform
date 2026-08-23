# CashFlow Desktop

Local-first Cash Flow MVP for Windows and macOS. The desktop UI is Electron + React; the local API is Go + SQLite.

## Development

1. Install Node dependencies: `pnpm install`
2. Start the Go API: `cd services/cashflow-api && go run ./cmd/api -db ./cashflow.db`
3. Start the desktop shell: `pnpm dev`

The API binds to `127.0.0.1` only. In production Electron starts it and keeps the renderer behind a narrow IPC bridge.

## Security

Passwords and recovery codes are hashed with Argon2id. CSV exports intentionally exclude credentials and secrets. Keep the one-time recovery code in a password manager.

