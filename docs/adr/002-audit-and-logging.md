# ADR 002: Correlatable audit and logs

Every user action gets a correlation ID at the Electron boundary. The ID is propagated through the local API, structured logs, and immutable audit events. Logs and exports exclude all credentials and secrets.

