package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/cashflow/desktop/api/internal/domain"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type mcpIdentityKey struct{}
type mcpIdentity struct {
	user          domain.User
	key           domain.MCPAPIKey
	clientAddress string
	clientAgent   string
}
type createTransactionInput struct {
	AccountID    string `json:"accountId" jsonschema:"ID de la cuenta activa"`
	CategoryID   string `json:"categoryId,omitempty" jsonschema:"ID opcional de una categoría existente"`
	CategoryName string `json:"categoryName,omitempty" jsonschema:"Nombre opcional: se compara con las categorías existentes, pero no crea categorías automáticamente"`
	Direction    string `json:"direction" jsonschema:"Tipo de movimiento: income o expense"`
	AmountMinor  int64  `json:"amountMinor" jsonschema:"Monto en unidades menores"`
	Currency     string `json:"currency" jsonschema:"Moneda ISO de tres letras"`
	Description  string `json:"description"`
	OccurredOn   string `json:"occurredOn" jsonschema:"Fecha YYYY-MM-DD"`
}
type createCategoryInput struct {
	Name      string `json:"name" jsonschema:"Nombre de la categoría"`
	Direction string `json:"direction" jsonschema:"Dirección: income, expense o both"`
	Confirmed bool   `json:"confirmed" jsonschema:"Debe ser true sólo después de confirmar que no se reutilizará una categoría existente"`
}

func (s *Server) mcpHTTP() http.Handler {
	s.mcpOnce.Do(func() {
		s.mcpHandler = mcp.NewStreamableHTTPHandler(func(request *http.Request) *mcp.Server {
			identity, ok := request.Context().Value(mcpIdentityKey{}).(mcpIdentity)
			if !ok {
				return nil
			}
			return s.mcpServer(identity)
		}, &mcp.StreamableHTTPOptions{Stateless: true, Logger: s.Log})
	})
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
		identity, err := s.mcpIdentity(r.Context(), r.Header.Get("Authorization"), mcpClientAddress(r), r.UserAgent())
		if err != nil {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		s.mcpHandler.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), mcpIdentityKey{}, identity)))
	})
}
func mcpClientAddress(request *http.Request) string {
	if os.Getenv("CASHFLOW_TRUST_PROXY") == "true" {
		if forwarded := strings.TrimSpace(strings.Split(request.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
			return forwarded
		}
	}
	return request.RemoteAddr
}
func (s *Server) mcpIdentity(ctx context.Context, authorization, remoteAddress, userAgent string) (mcpIdentity, error) {
	secret := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
	user, key, err := s.App.AuthenticateMCPAPIKey(ctx, secret)
	clientAddress, _, splitErr := net.SplitHostPort(remoteAddress)
	if splitErr != nil {
		clientAddress = remoteAddress
	}
	if len(userAgent) > 240 {
		userAgent = userAgent[:240]
	}
	return mcpIdentity{user: user, key: key, clientAddress: clientAddress, clientAgent: userAgent}, err
}
func (s *Server) recordMCPToolCall(ctx context.Context, identity mcpIdentity, tool string, input, result any, operationErr error) error {
	origin := map[string]string{"client_address": identity.clientAddress, "client_agent": identity.clientAgent}
	if auditErr := s.App.RecordMCPToolCall(ctx, identity.user, identity.key, tool, input, result, origin, operationErr); auditErr != nil {
		return auditErr
	}
	return operationErr
}
func (s *Server) mcpServer(identity mcpIdentity) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "cashflow", Version: "0.1.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "cashflow_list_accounts", Description: "Lista las cuentas activas de CashFlow."}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, map[string]any, error) {
		if !strings.Contains(identity.key.Scopes, "read") {
			err := errors.New("MCP key does not allow reading")
			return nil, nil, s.recordMCPToolCall(ctx, identity, "cashflow_list_accounts", map[string]any{}, nil, err)
		}
		values, err := s.App.Repo.ListAccounts(ctx)
		if err = s.recordMCPToolCall(ctx, identity, "cashflow_list_accounts", map[string]any{}, map[string]int{"count": len(values)}, err); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"accounts": values}, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "cashflow_list_categories", Description: "Lista las categorías activas de CashFlow."}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, map[string]any, error) {
		if !strings.Contains(identity.key.Scopes, "read") {
			err := errors.New("MCP key does not allow reading")
			return nil, nil, s.recordMCPToolCall(ctx, identity, "cashflow_list_categories", map[string]any{}, nil, err)
		}
		values, err := s.App.Repo.ListCategories(ctx)
		if err = s.recordMCPToolCall(ctx, identity, "cashflow_list_categories", map[string]any{}, map[string]int{"count": len(values)}, err); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"categories": values}, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "cashflow_create_category", Description: "Crea una categoría sólo después de que la persona confirmó que no desea reutilizar una existente. Antes usá cashflow_list_categories. Requiere una clave MCP con permiso de escritura."}, func(ctx context.Context, _ *mcp.CallToolRequest, input createCategoryInput) (*mcp.CallToolResult, map[string]any, error) {
		if !strings.Contains(identity.key.Scopes, "write") {
			err := errors.New("MCP key does not allow writing")
			return nil, nil, s.recordMCPToolCall(ctx, identity, "cashflow_create_category", input, nil, err)
		}
		if !input.Confirmed {
			categories, listErr := s.App.Repo.ListCategories(ctx)
			result := map[string]any{"created": false, "confirmationRequired": true, "requestedCategory": input.Name, "existingCategories": categories}
			if listErr != nil {
				err := s.recordMCPToolCall(ctx, identity, "cashflow_create_category", input, result, listErr)
				return nil, nil, err
			}
			if err := s.recordMCPToolCall(ctx, identity, "cashflow_create_category", input, result, nil); err != nil {
				return nil, nil, err
			}
			return nil, result, nil
		}
		err := s.App.CreateCategory(ctx, identity.user, input.Name, input.Direction, "mcp:"+identity.key.ID)
		result := map[string]any{"created": err == nil, "name": input.Name, "direction": input.Direction}
		if err = s.recordMCPToolCall(ctx, identity, "cashflow_create_category", input, result, err); err != nil {
			return nil, nil, err
		}
		return nil, result, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "cashflow_create_transaction", Description: "Crea un movimiento auditado. Sin categoría usa General income o General expense según el tipo. Si llega categoryName, primero informa las categorías existentes: no crea una nueva sin confirmación explícita mediante cashflow_create_category."}, func(ctx context.Context, _ *mcp.CallToolRequest, input createTransactionInput) (*mcp.CallToolResult, map[string]any, error) {
		if !strings.Contains(identity.key.Scopes, "write") {
			err := errors.New("MCP key does not allow writing")
			return nil, nil, s.recordMCPToolCall(ctx, identity, "cashflow_create_transaction", input, nil, err)
		}
		categoryID, resolution, err := s.resolveMCPTransactionCategory(ctx, input)
		if err != nil {
			if err = s.recordMCPToolCall(ctx, identity, "cashflow_create_transaction", input, resolution, err); err != nil {
				return nil, nil, err
			}
			return nil, resolution, nil
		}
		if categoryID == "" {
			result := map[string]any{"created": false, "category": resolution}
			if err = s.recordMCPToolCall(ctx, identity, "cashflow_create_transaction", input, result, nil); err != nil {
				return nil, nil, err
			}
			return nil, result, nil
		}
		tx := domain.Transaction{AccountID: input.AccountID, CategoryID: categoryID, Direction: input.Direction, AmountMinor: input.AmountMinor, Currency: input.Currency, Description: input.Description, OccurredOn: input.OccurredOn}
		err = s.App.CreateTransaction(ctx, identity.user, tx, "mcp:"+identity.key.ID)
		result := map[string]any{"transaction_id": tx.ID, "created": err == nil, "category": resolution}
		if err = s.recordMCPToolCall(ctx, identity, "cashflow_create_transaction", input, result, err); err != nil {
			return nil, nil, err
		}
		return nil, result, err
	})
	return server
}
func (s *Server) resolveMCPTransactionCategory(ctx context.Context, input createTransactionInput) (string, map[string]any, error) {
	if input.Direction != "income" && input.Direction != "expense" {
		return "", nil, errors.New("direction must be income or expense")
	}
	categories, err := s.App.Repo.ListCategories(ctx)
	if err != nil {
		return "", nil, err
	}
	if input.CategoryID != "" {
		for _, category := range categories {
			if category.ID == input.CategoryID {
				if category.Direction != "both" && category.Direction != input.Direction {
					return "", nil, errors.New("category direction does not match transaction direction")
				}
				return category.ID, map[string]any{"id": category.ID, "name": category.Name, "reused": true}, nil
			}
		}
		return "", nil, errors.New("categoryId does not match an active category")
	}
	name := strings.TrimSpace(input.CategoryName)
	if name != "" {
		for _, category := range categories {
			if strings.EqualFold(category.Name, name) {
				if category.Direction != "both" && category.Direction != input.Direction {
					return "", nil, errors.New("existing category direction does not match transaction direction")
				}
				return category.ID, map[string]any{"id": category.ID, "name": category.Name, "reused": true}, nil
			}
		}
		return "", map[string]any{"created": false, "confirmationRequired": true, "requestedCategory": name, "existingCategories": categories}, nil
	}
	defaultID := "seed-expense"
	defaultName := "General expense"
	if input.Direction == "income" {
		defaultID = "seed-income"
		defaultName = "General income"
	}
	for _, category := range categories {
		if category.ID == defaultID {
			return defaultID, map[string]any{"id": defaultID, "name": defaultName, "defaulted": true}, nil
		}
	}
	return "", nil, errors.New("default category is unavailable")
}
