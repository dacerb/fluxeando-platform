# ADR 001: Local-first SQLite

The MVP persists to a SQLite file owned by the operating-system user. The Go application is the only process allowed to access it. Repository interfaces isolate persistence so PostgreSQL can be introduced later without changing application services.

