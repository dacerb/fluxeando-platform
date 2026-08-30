package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/cashflow/desktop/api/internal/domain"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type mcpIdentityKey struct{}
type mcpIdentity struct {
	user domain.User
	key  domain.MCPAPIKey
}
type createTransactionInput struct {
	AccountID   string `json:"accountId" jsonschema:"required,description=ID de la cuenta activa"`
	CategoryID  string `json:"categoryId,omitempty" jsonschema:"description=ID opcional de la categoría"`
	Direction   string `json:"direction" jsonschema:"required,enum=income,enum=expense"`
	AmountMinor int64  `json:"amountMinor" jsonschema:"required,description=Monto en unidades menores"`
	Currency    string `json:"currency" jsonschema:"required,description=Moneda ISO de tres letras"`
	Description string `json:"description"`
	OccurredOn  string `json:"occurredOn" jsonschema:"required,description=Fecha YYYY-MM-DD"`
}

func (s *Server) mcpHTTP() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		settings, err := s.App.Repo.MCPSettings(r.Context())
		if err != nil || !settings.Enabled {
			http.NotFound(w, r)
			return
		}
		if settings.ExposureMode == "local" && r.Host != "" && !strings.HasPrefix(r.RemoteAddr, "127.0.0.1:") && !strings.HasPrefix(r.RemoteAddr, "[::1]:") {
			http.Error(w, "local MCP only", http.StatusForbidden)
			return
		}
		identity, err := s.mcpIdentity(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		mcp.NewStreamableHTTPHandler(func(request *http.Request) *mcp.Server { return s.mcpServer(identity) }, &mcp.StreamableHTTPOptions{Stateless: true, Logger: s.Log}).ServeHTTP(w, r)
	})
}
func (s *Server) mcpIdentity(ctx context.Context, authorization string) (mcpIdentity, error) {
	secret := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
	user, key, err := s.App.AuthenticateMCPAPIKey(ctx, secret)
	return mcpIdentity{user, key}, err
}
func (s *Server) mcpServer(identity mcpIdentity) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "cashflow", Version: "0.1.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "cashflow_list_accounts", Description: "Lista las cuentas activas de CashFlow."}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, map[string]any, error) {
		if !strings.Contains(identity.key.Scopes, "read") {
			return nil, nil, errors.New("MCP key does not allow reading")
		}
		values, err := s.App.Repo.ListAccounts(ctx)
		return nil, map[string]any{"accounts": values}, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "cashflow_list_categories", Description: "Lista las categorías activas de CashFlow."}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, map[string]any, error) {
		if !strings.Contains(identity.key.Scopes, "read") {
			return nil, nil, errors.New("MCP key does not allow reading")
		}
		values, err := s.App.Repo.ListCategories(ctx)
		return nil, map[string]any{"categories": values}, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "cashflow_create_transaction", Description: "Crea un movimiento. Esta acción queda auditada."}, func(ctx context.Context, _ *mcp.CallToolRequest, input createTransactionInput) (*mcp.CallToolResult, map[string]any, error) {
		if !strings.Contains(identity.key.Scopes, "write") {
			return nil, nil, errors.New("MCP key does not allow writing")
		}
		tx := domain.Transaction{AccountID: input.AccountID, CategoryID: input.CategoryID, Direction: input.Direction, AmountMinor: input.AmountMinor, Currency: input.Currency, Description: input.Description, OccurredOn: input.OccurredOn}
		err := s.App.CreateTransaction(ctx, identity.user, tx, "mcp:"+identity.key.ID)
		return nil, map[string]any{"created": err == nil}, err
	})
	return server
}
