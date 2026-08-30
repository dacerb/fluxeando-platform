package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/cashflow/desktop/api/internal/domain"
	"github.com/cashflow/desktop/api/internal/infrastructure/sqlite"
	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"
	"net/mail"
	"strings"
	"sync"
	"time"
)

type Service struct {
	Repo     *sqlite.Repository
	sessions map[string]domain.User
	mu       sync.RWMutex
}
type CategoryImport struct{ Name, Direction string }
type TransactionImport struct {
	AccountName, CategoryName, Direction, Currency, Description, OccurredOn string
	AmountMinor                                                             int64
}

func New(repo *sqlite.Repository) *Service {
	return &Service{Repo: repo, sessions: map[string]domain.User{}}
}
func hash(secret string) (string, error) {
	salt := make([]byte, 16)
	if _, e := rand.Read(salt); e != nil {
		return "", e
	}
	out := argon2.IDKey([]byte(secret), salt, 3, 64*1024, 4, 32)
	return base64.RawStdEncoding.EncodeToString(salt) + ":" + base64.RawStdEncoding.EncodeToString(out), nil
}
func verify(secret, encoded string) bool {
	p := strings.Split(encoded, ":")
	if len(p) != 2 {
		return false
	}
	salt, e := base64.RawStdEncoding.DecodeString(p[0])
	if e != nil {
		return false
	}
	want, e := base64.RawStdEncoding.DecodeString(p[1])
	if e != nil {
		return false
	}
	got := argon2.IDKey([]byte(secret), salt, 3, 64*1024, 4, uint32(len(want)))
	if len(got) != len(want) {
		return false
	}
	var d byte
	for i := range got {
		d |= got[i] ^ want[i]
	}
	return d == 0
}
func recoveryCode() (string, error) {
	b := make([]byte, 20)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func validatePassword(v string) error {
	if len(v) < 12 {
		return errors.New("password must contain at least 12 characters")
	}
	return nil
}
func validateInitialAdministrator(email, name, password string) error {
	email = strings.TrimSpace(email)
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || !strings.Contains(address.Address, "@") {
		return errors.New("a valid email address is required")
	}
	if strings.TrimSpace(name) == "" {
		return errors.New("display name is required")
	}
	if len(password) < 12 {
		return errors.New("password must contain at least 12 characters")
	}
	for _, character := range password {
		if character == ' ' || character == '\t' || character == '\n' || character == '\r' {
			return errors.New("password must not contain spaces")
		}
	}
	return nil
}
func (s *Service) Initialized(ctx context.Context) (bool, error) { return s.Repo.Initialized(ctx) }
func (s *Service) RecordPreferenceChange(ctx context.Context, actor domain.User, kind string, before, after map[string]string, correlation string) error {
	if kind != "appearance" && kind != "language" {
		return errors.New("unsupported preference change")
	}
	if len(after) == 0 {
		return errors.New("preference change is required")
	}
	return s.Repo.Audit(ctx, uuid.NewString(), actor.ID, kind+"_updated", "user_preference", actor.ID+":"+kind, correlation, before, after)
}
func (s *Service) SaveFilter(ctx context.Context, actor domain.User, name, query, correlation string) error {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(query) == "" {
		return errors.New("filter name and query are required")
	}
	filter := domain.SavedFilter{ID: uuid.NewString(), Name: strings.TrimSpace(name), Query: strings.TrimSpace(query)}
	if err := s.Repo.CreateSavedFilter(ctx, actor.ID, filter); err != nil {
		return err
	}
	return s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "saved_filter_created", "saved_filter", filter.ID, correlation, nil, map[string]string{"name": filter.Name})
}
func (s *Service) UpdateSavedFilter(ctx context.Context, actor domain.User, id, name, query, correlation string) error {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(query) == "" {
		return errors.New("filter name and query are required")
	}
	if err := s.Repo.UpdateSavedFilter(ctx, actor.ID, id, strings.TrimSpace(name), strings.TrimSpace(query)); err != nil {
		return err
	}
	return s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "saved_filter_updated", "saved_filter", id, correlation, nil, map[string]string{"name": strings.TrimSpace(name)})
}
func (s *Service) DeleteSavedFilter(ctx context.Context, actor domain.User, id, correlation string) error {
	if err := s.Repo.DeleteSavedFilter(ctx, actor.ID, id); err != nil {
		return err
	}
	return s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "saved_filter_deleted", "saved_filter", id, correlation, nil, nil)
}
func (s *Service) Initialize(ctx context.Context, email, name, password, correlation string) (string, error) {
	ok, e := s.Initialized(ctx)
	if e != nil {
		return "", e
	}
	if ok {
		return "", errors.New("application is already initialized")
	}
	if e = validateInitialAdministrator(email, name, password); e != nil {
		return "", e
	}
	ph, e := hash(password)
	if e != nil {
		return "", e
	}
	code, e := recoveryCode()
	if e != nil {
		return "", e
	}
	rh, e := hash(code)
	if e != nil {
		return "", e
	}
	id := uuid.NewString()
	if e = s.Repo.CreateUser(ctx, id, strings.ToLower(email), name, ph, rh, string(domain.RoleAdministrator)); e != nil {
		return "", e
	}
	if e = s.Repo.EnsureDefaultCategories(ctx); e != nil {
		return "", e
	}
	e = s.Repo.Audit(ctx, uuid.NewString(), id, "administrator_initialized", "user", id, correlation, nil, map[string]string{"email": email})
	return code, e
}
func (s *Service) Login(ctx context.Context, email, password string, correlations ...string) (string, domain.User, error) {
	return s.login(ctx, email, password, false, correlations...)
}
func (s *Service) LoginRemembered(ctx context.Context, email, password string, correlations ...string) (string, domain.User, error) {
	return s.login(ctx, email, password, true, correlations...)
}
func (s *Service) login(ctx context.Context, email, password string, remember bool, correlations ...string) (string, domain.User, error) {
	u, h, _, e := s.Repo.UserCredentials(ctx, strings.ToLower(email))
	if e != nil || !verify(password, h) {
		return "", domain.User{}, errors.New("invalid email or password")
	}
	token := uuid.NewString()
	if remember {
		bytes := make([]byte, 32)
		if _, e = rand.Read(bytes); e != nil {
			return "", domain.User{}, e
		}
		token = base64.RawURLEncoding.EncodeToString(bytes)
		digest := sha256.Sum256([]byte(token))
		if e = s.Repo.CreateRememberedSession(ctx, base64.RawURLEncoding.EncodeToString(digest[:]), u.ID, time.Now().UTC().Add(30*24*time.Hour)); e != nil {
			return "", domain.User{}, e
		}
	}
	s.mu.Lock()
	s.sessions[token] = u
	s.mu.Unlock()
	correlation := ""
	if len(correlations) > 0 {
		correlation = correlations[0]
	}
	if e = s.Repo.Audit(ctx, uuid.NewString(), u.ID, "user_logged_in", "user", u.ID, correlation, nil, map[string]string{"email": u.Email}); e != nil {
		return "", domain.User{}, e
	}
	return token, u, nil
}
func (s *Service) Session(token string) (domain.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.sessions[token]
	if ok {
		return u, true
	}
	digest := sha256.Sum256([]byte(token))
	return s.Repo.RememberedSession(context.Background(), base64.RawURLEncoding.EncodeToString(digest[:]))
}
func (s *Service) Logout(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
	digest := sha256.Sum256([]byte(token))
	s.Repo.DeleteRememberedSession(context.Background(), base64.RawURLEncoding.EncodeToString(digest[:]))
}
func (s *Service) EnsureRecoveryCode(ctx context.Context, user domain.User, correlation string) (string, error) {
	if user.Role != string(domain.RoleAdministrator) {
		return "", nil
	}
	_, _, recoveryHash, err := s.Repo.UserCredentials(ctx, user.Email)
	if err != nil {
		return "", err
	}
	if recoveryHash != "" {
		return "", nil
	}
	code, err := recoveryCode()
	if err != nil {
		return "", err
	}
	hash, err := hash(code)
	if err != nil {
		return "", err
	}
	if err = s.Repo.UpdateRecoveryCode(ctx, user.ID, hash); err != nil {
		return "", err
	}
	if err = s.Repo.Audit(ctx, uuid.NewString(), user.ID, "recovery_code_issued", "user", user.ID, correlation, nil, nil); err != nil {
		return "", err
	}
	return code, nil
}
func (s *Service) Recover(ctx context.Context, email, code, newPassword, correlation string) (string, error) {
	if e := validatePassword(newPassword); e != nil {
		return "", e
	}
	u, _, rh, e := s.Repo.UserCredentials(ctx, strings.ToLower(email))
	if e != nil || rh == "" || !verify(code, rh) {
		return "", errors.New("invalid recovery credentials")
	}
	h, e := hash(newPassword)
	if e != nil {
		return "", e
	}
	nextCode, e := recoveryCode()
	if e != nil {
		return "", e
	}
	nextRecoveryHash, e := hash(nextCode)
	if e != nil {
		return "", e
	}
	if e = s.Repo.UpdatePasswordWithRecovery(ctx, u.ID, h, nextRecoveryHash); e != nil {
		return "", e
	}
	if e = s.Repo.Audit(ctx, uuid.NewString(), u.ID, "password_recovered", "user", u.ID, correlation, nil, map[string]string{"email": u.Email}); e != nil {
		return "", e
	}
	return nextCode, nil
}
func (s *Service) Require(u domain.User, roles ...domain.Role) error {
	for _, r := range roles {
		if u.Role == string(r) {
			return nil
		}
	}
	return errors.New("forbidden")
}
func (s *Service) CreateUser(ctx context.Context, actor domain.User, email, name, password, role, correlation string) error {
	return s.CreateUserWithPasswordPolicy(ctx, actor, email, name, password, role, false, correlation)
}

func (s *Service) CreateUserWithPasswordPolicy(ctx context.Context, actor domain.User, email, name, password, role string, forcePasswordChange bool, correlation string) error {
	if e := s.Require(actor, domain.RoleAdministrator); e != nil {
		return e
	}
	if role != string(domain.RoleAdministrator) && role != string(domain.RoleManager) && role != string(domain.RoleOperator) {
		return errors.New("invalid role")
	}
	if e := validatePassword(password); e != nil {
		return e
	}
	h, e := hash(password)
	if e != nil {
		return e
	}
	id := uuid.NewString()
	if e = s.Repo.CreateUser(ctx, id, strings.ToLower(email), name, h, "", role); e != nil {
		return e
	}
	if forcePasswordChange {
		if e = s.Repo.UpdatePassword(ctx, id, h, true); e != nil {
			return e
		}
	}
	return s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "user_created", "user", id, correlation, nil, map[string]any{"email": email, "role": role, "passwordGenerated": forcePasswordChange, "mustChangePassword": forcePasswordChange})
}

func (s *Service) ChangeOwnPassword(ctx context.Context, actor domain.User, currentPassword, newPassword, correlation string) error {
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	_, currentHash, _, err := s.Repo.UserCredentials(ctx, actor.Email)
	if err != nil || !verify(currentPassword, currentHash) {
		return errors.New("current password is invalid")
	}
	h, err := hash(newPassword)
	if err != nil {
		return err
	}
	if err = s.Repo.UpdatePassword(ctx, actor.ID, h, false); err != nil {
		return err
	}
	s.mu.Lock()
	for token, user := range s.sessions {
		if user.ID == actor.ID {
			user.MustChangePassword = false
			s.sessions[token] = user
		}
	}
	s.mu.Unlock()
	return s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "password_changed", "user", actor.ID, correlation, nil, map[string]any{"forced": false})
}

func (s *Service) ResetUserPassword(ctx context.Context, actor domain.User, id, newPassword, correlation string) error {
	if err := s.Require(actor, domain.RoleAdministrator); err != nil {
		return err
	}
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	target, err := s.Repo.User(ctx, id)
	if err != nil {
		return err
	}
	h, err := hash(newPassword)
	if err != nil {
		return err
	}
	if err = s.Repo.UpdatePassword(ctx, id, h, true); err != nil {
		return err
	}
	s.invalidateUserSessions(id)
	return s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "password_reset_by_administrator", "user", id, correlation, nil, map[string]any{"email": target.Email, "mustChangePassword": true})
}
func (s *Service) invalidateUserSessions(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, user := range s.sessions {
		if user.ID == userID {
			delete(s.sessions, token)
		}
	}
	s.Repo.DeleteRememberedSessionsForUser(context.Background(), userID)
}
func (s *Service) validateUserAdministration(ctx context.Context, actor domain.User, id string) (domain.User, error) {
	if err := s.Require(actor, domain.RoleAdministrator); err != nil {
		return domain.User{}, err
	}
	if actor.ID == id {
		return domain.User{}, errors.New("you cannot deactivate or delete your own user")
	}
	target, err := s.Repo.User(ctx, id)
	if err != nil {
		return domain.User{}, err
	}
	if target.Role == string(domain.RoleAdministrator) && target.Active {
		count, err := s.Repo.ActiveAdministratorCount(ctx)
		if err != nil {
			return domain.User{}, err
		}
		if count <= 1 {
			return domain.User{}, errors.New("the last active administrator cannot be removed")
		}
	}
	return target, nil
}
func (s *Service) DeactivateUser(ctx context.Context, actor domain.User, id, correlation string) error {
	target, err := s.validateUserAdministration(ctx, actor, id)
	if err != nil {
		return err
	}
	if !target.Active {
		return errors.New("user is already inactive")
	}
	if err = s.Repo.SetUserActive(ctx, id, false); err != nil {
		return err
	}
	s.invalidateUserSessions(id)
	return s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "user_deactivated", "user", id, correlation, target, map[string]bool{"active": false})
}
func (s *Service) ActivateUser(ctx context.Context, actor domain.User, id, correlation string) error {
	if err := s.Require(actor, domain.RoleAdministrator); err != nil {
		return err
	}
	target, err := s.Repo.User(ctx, id)
	if err != nil {
		return err
	}
	if target.Active {
		return errors.New("user is already active")
	}
	if err = s.Repo.SetUserActive(ctx, id, true); err != nil {
		return err
	}
	return s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "user_activated", "user", id, correlation, target, map[string]bool{"active": true})
}
func (s *Service) DeleteUser(ctx context.Context, actor domain.User, id, correlation string) error {
	target, err := s.validateUserAdministration(ctx, actor, id)
	if err != nil {
		return err
	}
	if err = s.Repo.DeleteUser(ctx, id); err != nil {
		return errors.New("cannot delete a user with recorded activity; deactivate the user instead")
	}
	s.invalidateUserSessions(id)
	return s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "user_deleted", "user", id, correlation, target, nil)
}
func (s *Service) CreateAccount(ctx context.Context, actor domain.User, name, typ, correlation string) error {
	if e := s.Require(actor, domain.RoleAdministrator, domain.RoleManager); e != nil {
		return e
	}
	name = strings.TrimSpace(name)
	if name == "" || (typ != "cash" && typ != "bank" && typ != "wallet") {
		return errors.New("name and valid account type are required")
	}
	a := domain.Account{ID: uuid.NewString(), Name: name, Type: typ}
	if e := s.Repo.CreateAccount(ctx, a); e != nil {
		return e
	}
	return s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "account_created", "account", a.ID, correlation, nil, a)
}
func (s *Service) UpdateAccount(ctx context.Context, actor domain.User, id, name, typ, correlation string) error {
	if e := s.Require(actor, domain.RoleAdministrator, domain.RoleManager); e != nil {
		return e
	}
	name = strings.TrimSpace(name)
	if id == "" || name == "" || (typ != "cash" && typ != "bank" && typ != "wallet") {
		return errors.New("name and valid account type are required")
	}
	before, e := s.Repo.Account(ctx, id)
	if e != nil || !before.Active {
		return errors.New("account is inactive or does not exist")
	}
	after := domain.Account{ID: id, Name: name, Type: typ, Active: true}
	if e = s.Repo.UpdateAccount(ctx, after); e != nil {
		return e
	}
	return s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "account_updated", "account", id, correlation, before, after)
}
func (s *Service) DeactivateAccount(ctx context.Context, actor domain.User, id, correlation string) error {
	if e := s.Require(actor, domain.RoleAdministrator, domain.RoleManager); e != nil {
		return e
	}
	before, e := s.Repo.Account(ctx, id)
	if e != nil || !before.Active {
		return errors.New("account is inactive or does not exist")
	}
	if e = s.Repo.DeactivateAccount(ctx, id); e != nil {
		return e
	}
	after := before
	after.Active = false
	return s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "account_deactivated", "account", id, correlation, before, after)
}
func (s *Service) ActivateAccount(ctx context.Context, actor domain.User, id, correlation string) error {
	if err := s.Require(actor, domain.RoleAdministrator, domain.RoleManager); err != nil {
		return err
	}
	before, err := s.Repo.Account(ctx, id)
	if err != nil || before.Active {
		return errors.New("account is active or does not exist")
	}
	if err = s.Repo.ActivateAccount(ctx, id); err != nil {
		return err
	}
	after := before
	after.Active = true
	return s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "account_activated", "account", id, correlation, before, after)
}
func (s *Service) DeleteAccount(ctx context.Context, actor domain.User, id, replacementID, correlation string) error {
	if err := s.Require(actor, domain.RoleAdministrator, domain.RoleManager); err != nil {
		return err
	}
	before, err := s.Repo.Account(ctx, id)
	if err != nil || before.Active {
		return errors.New("deactivate the account before deleting it")
	}
	usage, err := s.Repo.AccountUsageCount(ctx, id)
	if err != nil {
		return err
	}
	if usage > 0 {
		if replacementID == "" {
			return errors.New("account has recorded movements; select a replacement account")
		}
		if replacementID == id {
			return errors.New("replacement account must be different")
		}
		replacement, err := s.Repo.Account(ctx, replacementID)
		if err != nil || !replacement.Active {
			return errors.New("replacement account is inactive or does not exist")
		}
		if err = s.Repo.MigrateAccountTransactions(ctx, id, replacementID); err != nil {
			return err
		}
		if err = s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "account_movements_migrated", "account", id, correlation, map[string]any{"from": id, "count": usage}, map[string]any{"to": replacementID, "count": usage}); err != nil {
			return err
		}
	}
	if err = s.Repo.DeleteAccount(ctx, id); err != nil {
		return err
	}
	return s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "account_deleted", "account", id, correlation, before, nil)
}
func (s *Service) CreateCategory(ctx context.Context, actor domain.User, name, direction, correlation string) error {
	if e := s.Require(actor, domain.RoleAdministrator, domain.RoleManager, domain.RoleOperator); e != nil {
		return e
	}
	name = strings.TrimSpace(name)
	if name == "" || (direction != "income" && direction != "expense" && direction != "both") {
		return errors.New("name and valid category direction are required")
	}
	c := domain.Category{ID: uuid.NewString(), Name: name, Direction: direction}
	if e := s.Repo.CreateCategory(ctx, c); e != nil {
		return e
	}
	return s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "category_created", "category", c.ID, correlation, nil, c)
}

func (s *Service) DeleteCategory(ctx context.Context, actor domain.User, id, replacementID, correlation string) error {
	if err := s.Require(actor, domain.RoleAdministrator, domain.RoleManager); err != nil {
		return err
	}
	before, err := s.Repo.Category(ctx, id)
	if err != nil || before.Active {
		return errors.New("deactivate the category before deleting it")
	}
	usage, err := s.Repo.CategoryUsageCount(ctx, id)
	if err != nil {
		return err
	}
	if usage > 0 {
		if replacementID == "" {
			return errors.New("category has recorded movements; select a replacement category")
		}
		if replacementID == id {
			return errors.New("replacement category must be different")
		}
		replacement, err := s.Repo.Category(ctx, replacementID)
		if err != nil || !replacement.Active {
			return errors.New("replacement category is inactive or does not exist")
		}
		if err = s.Repo.MigrateCategoryTransactions(ctx, id, replacementID); err != nil {
			return err
		}
	}
	if err = s.Repo.DeleteCategory(ctx, id); err != nil {
		return err
	}
	return s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "category_deleted", "category", id, correlation, before, nil)
}
func (s *Service) ImportCategories(ctx context.Context, actor domain.User, rows []CategoryImport, correlation string) error {
	if e := s.Require(actor, domain.RoleAdministrator, domain.RoleManager, domain.RoleOperator); e != nil {
		return e
	}
	if len(rows) == 0 {
		return errors.New("CSV has no data rows")
	}
	for index, row := range rows {
		if e := s.CreateCategory(ctx, actor, row.Name, row.Direction, correlation); e != nil {
			return fmt.Errorf("row %d: %w", index+2, e)
		}
	}
	return nil
}
func (s *Service) UpdateCategory(ctx context.Context, actor domain.User, id, name, direction, correlation string) error {
	if e := s.Require(actor, domain.RoleAdministrator, domain.RoleManager); e != nil {
		return e
	}
	name = strings.TrimSpace(name)
	if id == "" || name == "" || (direction != "income" && direction != "expense" && direction != "both") {
		return errors.New("name and valid category direction are required")
	}
	before, e := s.Repo.Category(ctx, id)
	if e != nil || !before.Active {
		return errors.New("category is inactive or does not exist")
	}
	after := domain.Category{ID: id, Name: name, Direction: direction, Active: true}
	if e = s.Repo.UpdateCategory(ctx, after); e != nil {
		return e
	}
	return s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "category_updated", "category", id, correlation, before, after)
}
func (s *Service) DeactivateCategory(ctx context.Context, actor domain.User, id, replacementID, correlation string) error {
	if e := s.Require(actor, domain.RoleAdministrator, domain.RoleManager); e != nil {
		return e
	}
	before, e := s.Repo.Category(ctx, id)
	if e != nil || !before.Active {
		return errors.New("category is inactive or does not exist")
	}
	usage, e := s.Repo.CategoryUsageCount(ctx, id)
	if e != nil {
		return e
	}
	if usage > 0 {
		if replacementID == "" {
			return errors.New("category is assigned to movements; select a replacement category")
		}
		if replacementID == id {
			return errors.New("replacement category must be different")
		}
		replacement, e := s.Repo.Category(ctx, replacementID)
		if e != nil || !replacement.Active {
			return errors.New("replacement category is inactive or does not exist")
		}
		if e = s.Repo.MigrateCategoryTransactions(ctx, id, replacementID); e != nil {
			return e
		}
		if e = s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "category_movements_migrated", "category", id, correlation, map[string]any{"from": id, "count": usage}, map[string]any{"to": replacementID, "count": usage}); e != nil {
			return e
		}
	}
	if e = s.Repo.DeactivateCategory(ctx, id); e != nil {
		return e
	}
	after := before
	after.Active = false
	return s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "category_deactivated", "category", id, correlation, before, after)
}
func (s *Service) ActivateCategory(ctx context.Context, actor domain.User, id, correlation string) error {
	if e := s.Require(actor, domain.RoleAdministrator, domain.RoleManager); e != nil {
		return e
	}
	before, e := s.Repo.Category(ctx, id)
	if e != nil || before.Active {
		return errors.New("category is active or does not exist")
	}
	if e = s.Repo.ActivateCategory(ctx, id); e != nil {
		return e
	}
	after := before
	after.Active = true
	return s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "category_activated", "category", id, correlation, before, after)
}
func (s *Service) CreateTransaction(ctx context.Context, actor domain.User, t domain.Transaction, correlation string) error {
	if e := s.Require(actor, domain.RoleAdministrator, domain.RoleManager, domain.RoleOperator); e != nil {
		return e
	}
	if t.AccountID == "" || t.AmountMinor <= 0 || len(t.Currency) != 3 || (t.Direction != "income" && t.Direction != "expense") || t.OccurredOn == "" {
		return errors.New("account, direction, positive amount, ISO currency and date are required")
	}
	if _, e := time.Parse("2006-01-02", t.OccurredOn); e != nil {
		return errors.New("occurred_on must be YYYY-MM-DD")
	}
	t.ID = uuid.NewString()
	t.CreatedBy = actor.ID
	if e := s.Repo.CreateTransaction(ctx, t); e != nil {
		return e
	}
	return s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "transaction_created", "transaction", t.ID, correlation, nil, t)
}
func (s *Service) ImportTransactions(ctx context.Context, actor domain.User, rows []TransactionImport, correlation string) error {
	if e := s.Require(actor, domain.RoleAdministrator, domain.RoleManager, domain.RoleOperator); e != nil {
		return e
	}
	if len(rows) == 0 {
		return errors.New("CSV has no data rows")
	}
	for index, row := range rows {
		account, e := s.Repo.AccountByName(ctx, strings.TrimSpace(row.AccountName))
		if e != nil {
			return fmt.Errorf("row %d: active account %q does not exist", index+2, row.AccountName)
		}
		categoryID := ""
		if strings.TrimSpace(row.CategoryName) != "" {
			categoryName := strings.TrimSpace(row.CategoryName)
			category, e := s.Repo.CategoryByName(ctx, categoryName)
			if e != nil {
				if e = s.CreateCategory(ctx, actor, categoryName, row.Direction, correlation); e != nil {
					return fmt.Errorf("row %d: create missing category %q: %w", index+2, categoryName, e)
				}
				category, e = s.Repo.CategoryByName(ctx, categoryName)
				if e != nil {
					return fmt.Errorf("row %d: load created category %q: %w", index+2, categoryName, e)
				}
			}
			categoryID = category.ID
		}
		t := domain.Transaction{AccountID: account.ID, CategoryID: categoryID, Direction: row.Direction, AmountMinor: row.AmountMinor, Currency: strings.ToUpper(strings.TrimSpace(row.Currency)), Description: row.Description, OccurredOn: row.OccurredOn}
		if e := s.CreateTransaction(ctx, actor, t, correlation); e != nil {
			return fmt.Errorf("row %d: %w", index+2, e)
		}
	}
	return nil
}
func (s *Service) VoidTransaction(ctx context.Context, actor domain.User, id, reason, correlation string) error {
	if actor.Role == string(domain.RoleOperator) {
		return s.RequestTransactionVoid(ctx, actor, id, reason, correlation)
	}
	if e := s.Require(actor, domain.RoleAdministrator, domain.RoleManager); e != nil {
		return e
	}
	if e := s.Repo.VoidTransaction(ctx, id); e != nil {
		return e
	}
	return s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "transaction_voided", "transaction", id, correlation, map[string]string{"status": "active"}, map[string]string{"status": "voided"})
}
func (s *Service) RequestTransactionVoid(ctx context.Context, actor domain.User, id, reason, correlation string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return errors.New("a deletion request reason is required")
	}
	items, err := s.Repo.ListTransactionsByCreator(ctx, "", "", actor.ID)
	if err != nil {
		return err
	}
	found := false
	for _, item := range items {
		if item.ID == id && item.Status == string(domain.TransactionActive) {
			found = true
			break
		}
	}
	if !found {
		return errors.New("you can only request voiding your active movements")
	}
	requestID := uuid.NewString()
	if err = s.Repo.CreateDeletionRequest(ctx, requestID, "transaction", id, actor.ID, actor.DisplayName, reason); err != nil {
		return err
	}
	return s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "transaction_void_requested", "deletion_request", requestID, correlation, nil, map[string]string{"transaction_id": id, "reason": reason})
}
func (s *Service) CancelDeletionRequest(ctx context.Context, actor domain.User, id, correlation string) error {
	request, err := s.Repo.DeletionRequest(ctx, id)
	if err != nil {
		return err
	}
	if request.RequestedBy != actor.ID {
		return errors.New("you can only cancel your own deletion request")
	}
	if request.Status != "pending" {
		return errors.New("deletion request is not pending")
	}
	if err = s.Repo.CancelDeletionRequest(ctx, id); err != nil {
		return err
	}
	return s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "deletion_request_cancelled", "deletion_request", id, correlation, request, map[string]string{"status": "cancelled"})
}
func (s *Service) ResolveDeletionRequest(ctx context.Context, actor domain.User, id, decision, correlation string) error {
	if e := s.Require(actor, domain.RoleAdministrator, domain.RoleManager); e != nil {
		return e
	}
	if decision != "approved" && decision != "rejected" {
		return errors.New("invalid deletion request decision")
	}
	request, err := s.Repo.DeletionRequest(ctx, id)
	if err != nil {
		return err
	}
	if request.Status != "pending" {
		return errors.New("deletion request is not pending")
	}
	if decision == "approved" && request.EntityType == "transaction" {
		if err = s.Repo.VoidTransaction(ctx, request.EntityID); err != nil {
			return err
		}
	}
	if err = s.Repo.ResolveDeletionRequest(ctx, id, decision, actor.ID); err != nil {
		return err
	}
	return s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "deletion_request_"+decision, "deletion_request", id, correlation, request, map[string]string{"status": decision})
}
func (s *Service) UpdateTransaction(ctx context.Context, actor domain.User, t domain.Transaction, correlation string) error {
	if e := s.Require(actor, domain.RoleAdministrator, domain.RoleManager, domain.RoleOperator); e != nil {
		return e
	}
	if t.ID == "" || t.AccountID == "" || t.AmountMinor <= 0 || len(t.Currency) != 3 || (t.Direction != "income" && t.Direction != "expense") {
		return errors.New("valid transaction values are required")
	}
	if _, e := time.Parse("2006-01-02", t.OccurredOn); e != nil {
		return errors.New("occurred_on must be YYYY-MM-DD")
	}
	all, e := s.Repo.ListTransactions(ctx, "", "")
	if e != nil {
		return e
	}
	var before domain.Transaction
	found := false
	for _, item := range all {
		if item.ID == t.ID {
			before = item
			found = true
			break
		}
	}
	if !found {
		return errors.New("transaction does not exist")
	}
	if e = s.Repo.UpdateTransaction(ctx, t); e != nil {
		return e
	}
	return s.Repo.Audit(ctx, uuid.NewString(), actor.ID, "transaction_updated", "transaction", t.ID, correlation, before, t)
}
