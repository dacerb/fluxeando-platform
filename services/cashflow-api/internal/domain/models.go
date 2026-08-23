package domain

type Role string

const (
	RoleAdministrator Role = "administrator"
	RoleManager       Role = "manager"
	RoleOperator      Role = "operator"
)

type TransactionStatus string

const (
	TransactionActive TransactionStatus = "active"
	TransactionVoided TransactionStatus = "voided"
)

type User struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	Active      bool   `json:"active"`
}
type DeletionRequest struct {
	ID              string `json:"id"`
	EntityType      string `json:"entityType"`
	EntityID        string `json:"entityId"`
	RequestedBy     string `json:"requestedBy"`
	RequestedByName string `json:"requestedByName"`
	Reason          string `json:"reason"`
	Status          string `json:"status"`
	ResolvedBy      string `json:"resolvedBy"`
	CreatedAt       string `json:"createdAt"`
	ResolvedAt      string `json:"resolvedAt"`
}
type AuditEvent struct {
	ID            string `json:"id"`
	ActorID       string `json:"actorId"`
	ActorName     string `json:"actorName"`
	Action        string `json:"action"`
	EntityType    string `json:"entityType"`
	EntityID      string `json:"entityId"`
	CorrelationID string `json:"correlationId"`
	Before        string `json:"before"`
	After         string `json:"after"`
	CreatedAt     string `json:"createdAt"`
}
type SavedFilter struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Query     string `json:"query"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}
type Account struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Active bool   `json:"active"`
}
type Category struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Direction  string `json:"direction"`
	Active     bool   `json:"active"`
	UsageCount int    `json:"usageCount"`
}
type Transaction struct {
	ID           string `json:"id"`
	AccountID    string `json:"accountId"`
	CategoryID   string `json:"categoryId"`
	CategoryName string `json:"categoryName"`
	Direction    string `json:"direction"`
	AmountMinor  int64  `json:"amountMinor"`
	Currency     string `json:"currency"`
	Description  string `json:"description"`
	OccurredOn   string `json:"occurredOn"`
	Status       string `json:"status"`
	CreatedBy    string `json:"createdBy"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}
