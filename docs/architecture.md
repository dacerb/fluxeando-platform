# Architecture

`Electron renderer -> preload IPC -> Electron main -> Go HTTP API -> application service -> repository -> SQLite`

The renderer never opens SQLite. Go routes are local-only and all protected operations require an in-memory bearer session. Each request receives or propagates `X-Correlation-ID`; the same ID is present in JSON logs and audit events.

## Logging convention

Every log is structured JSON with `timestamp`, `level`, `message`, `correlation_id`, `component`, `layer`, and `operation`. Never log passwords, recovery codes, tokens, or database credentials.

