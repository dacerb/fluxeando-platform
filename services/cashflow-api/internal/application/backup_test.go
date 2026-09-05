package application

import (
	"context"
	"fmt"
	"github.com/cashflow/desktop/api/internal/domain"
	"github.com/cashflow/desktop/api/internal/infrastructure/sqlite"
	"os"
	"path/filepath"
	"testing"
)

func TestRotateFilesystemKeepsAllFilesWhenRetentionExceedsExistingBackups(t *testing.T) {
	folder := t.TempDir()
	const prefix = "fluxeando-backup"
	for index := 0; index < 7; index++ {
		name := fmt.Sprintf("%s-20260904T00000%dZ.json", prefix, index)
		if err := os.WriteFile(filepath.Join(folder, name), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	manager := &BackupManager{}
	if err := manager.rotateFilesystem(folder, prefix, 30); err != nil {
		t.Fatalf("rotation with fewer files than retention must not fail: %v", err)
	}
	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 7 {
		t.Fatalf("rotation removed existing backups: got %d files, want 7", len(entries))
	}
}

func TestRunDoesNotCrashWhenRetentionExceedsExistingBackups(t *testing.T) {
	ctx := context.Background()
	repo, err := sqlite.Open(filepath.Join(t.TempDir(), "cashflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	folder := t.TempDir()
	const prefix = "fluxeando-backup"
	for index := 0; index < 7; index++ {
		name := fmt.Sprintf("%s-20260904T00000%dZ.json", prefix, index)
		if err := os.WriteFile(filepath.Join(folder, name), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.SaveBackupSettings(ctx, domain.BackupSettings{Provider: "filesystem", FilesystemPath: folder, FilenamePrefix: prefix, RetentionCount: 30, DelaySeconds: 60, BackupOnStartup: true, BackupOnShutdown: true}); err != nil {
		t.Fatal(err)
	}

	if err := NewBackupManager(repo, "", "", "", "", "", "").Run(ctx); err != nil {
		t.Fatalf("backup run must not fail when retention exceeds existing files: %v", err)
	}
	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 8 {
		t.Fatalf("backup run kept %d files, want 8", len(entries))
	}
}

func TestValidateBackupSettingsRequiresAtLeastSixtySeconds(t *testing.T) {
	err := ValidateBackupSettings(domain.BackupSettings{Provider: "filesystem", FilesystemPath: t.TempDir(), RetentionCount: 3, DelaySeconds: 59})
	if err == nil {
		t.Fatal("expected a delay shorter than 60 seconds to be rejected")
	}
	if err := ValidateBackupSettings(domain.BackupSettings{Provider: "filesystem", FilesystemPath: t.TempDir(), RetentionCount: 3, DelaySeconds: 60}); err != nil {
		t.Fatalf("60 seconds must be accepted: %v", err)
	}
}

func TestRotateFilesystemRemovesOnlyFilesPastRetention(t *testing.T) {
	folder := t.TempDir()
	const prefix = "fluxeando-backup"
	for index := 0; index < 4; index++ {
		name := fmt.Sprintf("%s-20260904T00000%dZ.json", prefix, index)
		if err := os.WriteFile(filepath.Join(folder, name), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	manager := &BackupManager{}
	if err := manager.rotateFilesystem(folder, prefix, 3); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("rotation kept %d files, want 3", len(entries))
	}
}
