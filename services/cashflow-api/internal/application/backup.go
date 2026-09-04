package application

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cashflow/desktop/api/internal/domain"
	"github.com/cashflow/desktop/api/internal/infrastructure/sqlite"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

const BackupDebounce = 10 * time.Second

// BackupManager owns the single delayed task for this process. Scheduling a
// new change always replaces the previous timer, so a burst creates one file.
type BackupManager struct {
	repo              *sqlite.Repository
	mu                sync.Mutex
	timer             *time.Timer
	root, googleToken string
	delay             time.Duration
	oauthConfig       *oauth2.Config
	cipher            cipher.AEAD
	states            map[string]time.Time
}

func NewBackupManager(repo *sqlite.Repository, root, googleToken, googleClientID, googleClientSecret, googleRedirectURL, encryptionKey string) *BackupManager {
	manager := &BackupManager{repo: repo, root: root, googleToken: googleToken, delay: BackupDebounce, states: map[string]time.Time{}}
	if googleClientID != "" && googleClientSecret != "" && googleRedirectURL != "" {
		manager.oauthConfig = &oauth2.Config{ClientID: googleClientID, ClientSecret: googleClientSecret, RedirectURL: googleRedirectURL, Endpoint: oauth2.Endpoint{AuthURL: "https://accounts.google.com/o/oauth2/v2/auth", TokenURL: "https://oauth2.googleapis.com/token"}, Scopes: []string{"https://www.googleapis.com/auth/drive.file"}}
		if raw, err := base64.RawStdEncoding.DecodeString(encryptionKey); err == nil && len(raw) == 32 {
			block, _ := aes.NewCipher(raw)
			manager.cipher, _ = cipher.NewGCM(block)
		}
	}
	return manager
}
func (m *BackupManager) BeginGoogleAuthorization() (string, error) {
	if m.oauthConfig == nil || m.cipher == nil {
		return "", errors.New("Google Drive OAuth is not configured on this server")
	}
	state := uuid.NewString()
	m.mu.Lock()
	m.states[state] = time.Now().Add(10 * time.Minute)
	m.mu.Unlock()
	return m.oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce), nil
}
func (m *BackupManager) CompleteGoogleAuthorization(ctx context.Context, state, code string) error {
	m.mu.Lock()
	expires, ok := m.states[state]
	delete(m.states, state)
	m.mu.Unlock()
	if !ok || time.Now().After(expires) {
		return errors.New("Google Drive authorization expired or is invalid")
	}
	token, err := m.oauthConfig.Exchange(ctx, code)
	if err != nil {
		return err
	}
	if token.RefreshToken == "" {
		return errors.New("Google Drive did not return a refresh token; revoke the previous grant and try again")
	}
	encrypted, err := m.encrypt(token.RefreshToken)
	if err != nil {
		return err
	}
	return m.repo.SaveGoogleBackupRefreshToken(ctx, encrypted)
}
func (m *BackupManager) GoogleConnected(ctx context.Context) bool {
	if m.googleToken != "" {
		return true
	}
	encrypted, ok, err := m.repo.GoogleBackupRefreshToken(ctx)
	return err == nil && ok && len(encrypted) > 0 && m.cipher != nil
}
func (m *BackupManager) Schedule() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.timer != nil {
		m.timer.Stop()
	}
	m.timer = time.AfterFunc(m.delay, func() { _ = m.Run(context.Background()) })
}
func (m *BackupManager) Run(ctx context.Context) error {
	settings, err := m.repo.BackupSettings(ctx)
	if err != nil {
		return err
	}
	if settings.Provider == "" {
		return nil
	}
	payload, err := m.snapshot(ctx)
	if err != nil {
		_ = m.repo.SaveBackupResult(ctx, "", err.Error())
		return err
	}
	prefix := safeFilenamePrefix(settings.FilenamePrefix)
	name := prefix + "-" + time.Now().UTC().Format("20060102T150405Z") + ".json"
	switch settings.Provider {
	case "filesystem":
		err = m.writeFilesystem(settings.FilesystemPath, name, payload)
	case "google_drive":
		err = m.writeGoogleDrive(ctx, settings.GoogleFolderID, name, payload)
	default:
		err = errors.New("unsupported backup provider")
	}
	if err != nil {
		_ = m.repo.SaveBackupResult(ctx, "", err.Error())
		return err
	}
	return m.repo.SaveBackupResult(ctx, time.Now().UTC().Format(time.RFC3339Nano), "")
}
func (m *BackupManager) snapshot(ctx context.Context) ([]byte, error) {
	// The audit log is deliberately included: a backup is a portable record,
	// independent of SQLite/MySQL internals.
	accounts, err := m.repo.ListAllAccounts(ctx)
	if err != nil {
		return nil, err
	}
	categories, err := m.repo.ListAllCategories(ctx)
	if err != nil {
		return nil, err
	}
	transactions, err := m.repo.ListTransactions(ctx, "", "")
	if err != nil {
		return nil, err
	}
	users, err := m.repo.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	audit, err := m.repo.ListAuditEvents(ctx)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(map[string]any{"format": "fluxeando-backup/v1", "createdAt": time.Now().UTC().Format(time.RFC3339Nano), "accounts": accounts, "categories": categories, "transactions": transactions, "users": users, "auditEvents": audit}, "", "  ")
}
func (m *BackupManager) writeFilesystem(target, name string, body []byte) error {
	if target == "" {
		return errors.New("backup folder is required")
	}
	if !filepath.IsAbs(target) {
		return errors.New("backup folder must be absolute")
	}
	if m.root != "" {
		relative, err := filepath.Rel(m.root, target)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			return errors.New("backup folder is outside the allowed backup root")
		}
	}
	if err := os.MkdirAll(target, 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(target, name), body, 0600)
}
func (m *BackupManager) writeGoogleDrive(ctx context.Context, folderID, name string, body []byte) error {
	accessToken, err := m.googleAccessToken(ctx)
	if err != nil {
		return err
	}
	if folderID == "" {
		return errors.New("Google Drive folder ID is required")
	}
	metadata, _ := json.Marshal(map[string]any{"name": name, "parents": []string{folderID}, "mimeType": "application/json"})
	boundary := "fluxeando-backup-boundary"
	var multipart bytes.Buffer
	fmt.Fprintf(&multipart, "--%s\r\nContent-Type: application/json; charset=UTF-8\r\n\r\n%s\r\n--%s\r\nContent-Type: application/json\r\n\r\n", boundary, metadata, boundary)
	multipart.Write(body)
	fmt.Fprintf(&multipart, "\r\n--%s--\r\n", boundary)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://www.googleapis.com/upload/drive/v3/files?uploadType=multipart", &multipart)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "multipart/related; boundary="+boundary)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Google Drive upload failed: %s", response.Status)
	}
	return nil
}
func (m *BackupManager) googleAccessToken(ctx context.Context) (string, error) {
	if m.googleToken != "" {
		return m.googleToken, nil
	}
	if m.oauthConfig == nil || m.cipher == nil {
		return "", errors.New("Google Drive OAuth is not configured on this server")
	}
	encrypted, ok, err := m.repo.GoogleBackupRefreshToken(ctx)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errors.New("Google Drive has not been authorized")
	}
	refreshToken, err := m.decrypt(encrypted)
	if err != nil {
		return "", err
	}
	token, err := m.oauthConfig.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken}).Token()
	if err != nil {
		return "", err
	}
	return token.AccessToken, nil
}
func (m *BackupManager) encrypt(value string) ([]byte, error) {
	nonce := make([]byte, m.cipher.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return append(nonce, m.cipher.Seal(nil, nonce, []byte(value), nil)...), nil
}
func (m *BackupManager) decrypt(value []byte) (string, error) {
	if m.cipher == nil || len(value) < m.cipher.NonceSize() {
		return "", errors.New("Google Drive credential cannot be decrypted")
	}
	nonce := value[:m.cipher.NonceSize()]
	decoded, err := m.cipher.Open(nil, nonce, value[m.cipher.NonceSize():], nil)
	return string(decoded), err
}
func ValidateBackupSettings(value domain.BackupSettings) error {
	if value.Provider == "" {
		return nil
	}
	if value.Provider != "filesystem" && value.Provider != "google_drive" {
		return errors.New("backup provider must be filesystem or google_drive")
	}
	if value.Provider == "filesystem" && strings.TrimSpace(value.FilesystemPath) == "" {
		return errors.New("backup folder is required")
	}
	if value.Provider == "google_drive" {
		if _, err := NormalizeGoogleFolderID(value.GoogleFolderID); err != nil {
			return err
		}
	}
	return nil
}
func NormalizeGoogleFolderID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("Google Drive folder ID is required")
	}
	if marker := "/folders/"; strings.Contains(value, marker) {
		value = strings.SplitN(value, marker, 2)[1]
		value = strings.SplitN(value, "?", 2)[0]
		value = strings.SplitN(value, "/", 2)[0]
	}
	if value == "" || strings.ContainsAny(value, " /?&#") {
		return "", errors.New("Google Drive folder ID or folder URL is invalid")
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '-' && r != '_' {
			return "", errors.New("Google Drive folder ID or folder URL is invalid")
		}
	}
	return value, nil
}
func safeFilenamePrefix(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "fluxeando-backup"
	}
	value = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return -1
	}, value)
	if value == "" {
		return "fluxeando-backup"
	}
	return value
}
