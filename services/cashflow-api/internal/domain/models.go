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
	ID          string `json:"id"`
	EntityType  string `json:"entityType"`
	EntityID    string `json:"entityId"`
	RequestedBy string `json:"requestedBy"`
	Status      string `json:"status"`
	CreatedAt   string `json:"createdAt"`
}
type Account struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Active bool   `json:"active"`
}
type Category struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Direction string `json:"direction"`
	Active    bool   `json:"active"`
}
type Transaction struct {
	ID          string `json:"id"`
	AccountID   string `json:"accountId"`
	CategoryID  string `json:"categoryId"`
	Direction   string `json:"direction"`
	AmountMinor int64  `json:"amountMinor"`
	Currency    string `json:"currency"`
	Description string `json:"description"`
	OccurredOn  string `json:"occurredOn"`
	Status      string `json:"status"`
	CreatedBy   string `json:"createdBy"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}
