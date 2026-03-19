package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFromEnvLocalModeCreatesWritableUploadDir(t *testing.T) {
	uploadDir := filepath.Join(t.TempDir(), "testdata", "uploads")
	setValidWebEnv(t, uploadDir)

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.Storage.Mode != UploadStorageModeLocal {
		t.Fatalf("expected storage mode %q, got %q", UploadStorageModeLocal, cfg.Storage.Mode)
	}

	if cfg.Storage.LocalDir != uploadDir {
		t.Fatalf("expected storage local dir %q, got %q", uploadDir, cfg.Storage.LocalDir)
	}
	if cfg.DatabasePath != "./var/app.db" {
		t.Fatalf("expected database path fallback from DATABASE_PATH, got %q", cfg.DatabasePath)
	}
	if cfg.DatabaseURL != "file:./var/app.db" {
		t.Fatalf("expected local database URL fallback from DATABASE_PATH, got %q", cfg.DatabaseURL)
	}
	if !cfg.DatabaseIsLocal {
		t.Fatal("expected local database config")
	}
	if cfg.DatabaseAuthToken != "" {
		t.Fatalf("expected no local auth token, got %q", cfg.DatabaseAuthToken)
	}
	if cfg.BetterstackToken != "" {
		t.Fatalf("expected empty betterstack token by default, got %q", cfg.BetterstackToken)
	}
	if cfg.BetterstackEndpoint != "" {
		t.Fatalf("expected empty betterstack endpoint by default, got %q", cfg.BetterstackEndpoint)
	}

	if _, err := os.Stat(uploadDir); err != nil {
		t.Fatalf("expected upload directory to exist: %v", err)
	}

	probeFile := filepath.Join(uploadDir, "probe.txt")
	if err := os.WriteFile(probeFile, []byte("ok"), 0o644); err != nil {
		t.Fatalf("expected upload directory to be writable: %v", err)
	}
}

func TestLoadFromEnvFailsWhenRequiredVarMissing(t *testing.T) {
	setValidWebEnv(t, filepath.Join(t.TempDir(), "uploads"))
	t.Setenv("DATABASE_PATH", "")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error for missing DATABASE_PATH")
	}

	if !strings.Contains(err.Error(), "DATABASE_PATH is required when DATABASE_URL is not set in local mode") {
		t.Fatalf("expected actionable error for DATABASE_PATH, got: %v", err)
	}
}

func TestLoadFromEnvUsesExplicitRemoteDatabaseURL(t *testing.T) {
	setValidWebEnv(t, filepath.Join(t.TempDir(), "unused"))
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_PATH", "")
	t.Setenv("DATABASE_URL", "libsql://example-org.turso.io")
	t.Setenv("DATABASE_AUTH_TOKEN", "secret-token")
	t.Setenv("UPLOAD_STORAGE_MODE", UploadStorageModeUploadThing)
	t.Setenv("UPLOAD_LOCAL_DIR", "")
	t.Setenv("UPLOADTHING_TOKEN", "uploadthing-token")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("expected remote database configuration to load, got: %v", err)
	}

	if cfg.DatabaseIsLocal {
		t.Fatal("expected remote database config")
	}
	if cfg.DatabaseURL != "libsql://example-org.turso.io" {
		t.Fatalf("expected remote database URL to be preserved, got %q", cfg.DatabaseURL)
	}
	if cfg.DatabaseAuthToken != "secret-token" {
		t.Fatalf("expected remote auth token to be preserved, got %q", cfg.DatabaseAuthToken)
	}
}

func TestLoadFromEnvRejectsNonLocalEnvWithoutDatabaseURL(t *testing.T) {
	setValidWebEnv(t, filepath.Join(t.TempDir(), "unused"))
	t.Setenv("APP_ENV", "staging")
	t.Setenv("DATABASE_PATH", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("UPLOAD_STORAGE_MODE", UploadStorageModeUploadThing)
	t.Setenv("UPLOAD_LOCAL_DIR", "")
	t.Setenv("UPLOADTHING_TOKEN", "uploadthing-token")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error when APP_ENV is not local and DATABASE_URL is missing")
	}

	if !strings.Contains(err.Error(), "DATABASE_URL is required when APP_ENV is not local") {
		t.Fatalf("expected actionable DATABASE_URL error, got: %v", err)
	}
}

func TestLoadFromEnvRejectsRemoteDatabaseWithoutAuthToken(t *testing.T) {
	setValidWebEnv(t, filepath.Join(t.TempDir(), "unused"))
	t.Setenv("APP_ENV", "staging")
	t.Setenv("DATABASE_PATH", "")
	t.Setenv("DATABASE_URL", "libsql://example-org.turso.io")
	t.Setenv("DATABASE_AUTH_TOKEN", "")
	t.Setenv("UPLOAD_STORAGE_MODE", UploadStorageModeUploadThing)
	t.Setenv("UPLOAD_LOCAL_DIR", "")
	t.Setenv("UPLOADTHING_TOKEN", "uploadthing-token")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error when remote DATABASE_URL is set without DATABASE_AUTH_TOKEN")
	}

	if !strings.Contains(err.Error(), "DATABASE_AUTH_TOKEN is required for remote DATABASE_URL") {
		t.Fatalf("expected actionable DATABASE_AUTH_TOKEN error, got: %v", err)
	}
}

func TestLoadFromEnvRejectsAuthTokenForLocalDatabase(t *testing.T) {
	setValidWebEnv(t, filepath.Join(t.TempDir(), "uploads"))
	t.Setenv("DATABASE_AUTH_TOKEN", "should-not-be-set")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error when local mode provides DATABASE_AUTH_TOKEN")
	}

	if !strings.Contains(err.Error(), "DATABASE_AUTH_TOKEN must be empty for local database connections") {
		t.Fatalf("expected actionable local auth token error, got: %v", err)
	}
}

func TestConfigDatabaseDSNUsesLocalURLUnchanged(t *testing.T) {
	cfg := Config{
		DatabaseURL:     "file:./var/app.db",
		DatabaseIsLocal: true,
	}

	if got := cfg.DatabaseDSN(); got != "file:./var/app.db" {
		t.Fatalf("expected local DSN to remain unchanged, got %q", got)
	}
}

func TestConfigDatabaseDSNAppendsAuthTokenForRemoteURL(t *testing.T) {
	cfg := Config{
		DatabaseURL:       "libsql://example-org.turso.io",
		DatabaseAuthToken: "secret-token",
		DatabaseIsLocal:   false,
	}

	if got := cfg.DatabaseDSN(); got != "libsql://example-org.turso.io?authToken=secret-token" {
		t.Fatalf("expected auth token to be appended to remote DSN, got %q", got)
	}
}

func TestConfigDatabaseDSNPreservesExistingRemoteAuthToken(t *testing.T) {
	cfg := Config{
		DatabaseURL:       "libsql://example-org.turso.io?authToken=existing-token",
		DatabaseAuthToken: "secret-token",
		DatabaseIsLocal:   false,
	}

	if got := cfg.DatabaseDSN(); got != "libsql://example-org.turso.io?authToken=existing-token" {
		t.Fatalf("expected existing auth token query to be preserved, got %q", got)
	}
}

func TestLoadFromEnvAcceptsUploadThingWithoutLocalDir(t *testing.T) {
	setValidWebEnv(t, filepath.Join(t.TempDir(), "unused"))
	t.Setenv("UPLOAD_STORAGE_MODE", UploadStorageModeUploadThing)
	t.Setenv("UPLOAD_LOCAL_DIR", "")
	t.Setenv("UPLOADTHING_TOKEN", "uploadthing-token")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("expected uploadthing mode to load, got: %v", err)
	}

	if cfg.Storage.Mode != UploadStorageModeUploadThing {
		t.Fatalf("expected storage mode %q, got %q", UploadStorageModeUploadThing, cfg.Storage.Mode)
	}

	if cfg.Storage.LocalDir != "" {
		t.Fatalf("expected empty local dir in uploadthing mode, got %q", cfg.Storage.LocalDir)
	}
	if cfg.Storage.UploadThingToken != "uploadthing-token" {
		t.Fatalf("expected uploadthing token to be loaded, got %q", cfg.Storage.UploadThingToken)
	}
	if cfg.Storage.UploadThingPrepareUploadURL != "https://api.uploadthing.com/v7/prepareUpload" {
		t.Fatalf("expected default uploadthing prepare URL, got %q", cfg.Storage.UploadThingPrepareUploadURL)
	}
	if cfg.Storage.UploadThingRequestTimeoutSecs != 30 {
		t.Fatalf("expected default uploadthing timeout 30s, got %d", cfg.Storage.UploadThingRequestTimeoutSecs)
	}
}

func TestLoadFromEnvRejectsUnsupportedStorageMode(t *testing.T) {
	setValidWebEnv(t, filepath.Join(t.TempDir(), "uploads"))
	t.Setenv("UPLOAD_STORAGE_MODE", "s3")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error for unsupported UPLOAD_STORAGE_MODE")
	}

	if !strings.Contains(err.Error(), "UPLOAD_STORAGE_MODE must be one of") {
		t.Fatalf("expected actionable storage mode error, got: %v", err)
	}
}

func TestLoadFromEnvRejectsUploadThingWithoutToken(t *testing.T) {
	setValidWebEnv(t, filepath.Join(t.TempDir(), "unused"))
	t.Setenv("UPLOAD_STORAGE_MODE", UploadStorageModeUploadThing)
	t.Setenv("UPLOAD_LOCAL_DIR", "")
	t.Setenv("UPLOADTHING_TOKEN", "")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error when uploadthing mode has empty UPLOADTHING_TOKEN")
	}

	if !strings.Contains(err.Error(), "UPLOADTHING_TOKEN is required when UPLOAD_STORAGE_MODE=uploadthing") {
		t.Fatalf("expected actionable uploadthing token error, got: %v", err)
	}
}

func TestLoadFromEnvRejectsInvalidUploadThingPrepareUploadURL(t *testing.T) {
	setValidWebEnv(t, filepath.Join(t.TempDir(), "unused"))
	t.Setenv("UPLOAD_STORAGE_MODE", UploadStorageModeUploadThing)
	t.Setenv("UPLOAD_LOCAL_DIR", "")
	t.Setenv("UPLOADTHING_TOKEN", "uploadthing-token")
	t.Setenv("UPLOADTHING_PREPARE_UPLOAD_URL", "://bad-url")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error for invalid UPLOADTHING_PREPARE_UPLOAD_URL")
	}

	if !strings.Contains(err.Error(), "UPLOADTHING_PREPARE_UPLOAD_URL must be a valid URL") {
		t.Fatalf("expected actionable uploadthing URL error, got: %v", err)
	}
}

func TestLoadFromEnvRejectsInvalidUploadThingRequestTimeout(t *testing.T) {
	setValidWebEnv(t, filepath.Join(t.TempDir(), "unused"))
	t.Setenv("UPLOAD_STORAGE_MODE", UploadStorageModeUploadThing)
	t.Setenv("UPLOAD_LOCAL_DIR", "")
	t.Setenv("UPLOADTHING_TOKEN", "uploadthing-token")
	t.Setenv("UPLOADTHING_REQUEST_TIMEOUT_SECS", "0")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error for invalid UPLOADTHING_REQUEST_TIMEOUT_SECS")
	}

	if !strings.Contains(err.Error(), "UPLOADTHING_REQUEST_TIMEOUT_SECS must be a positive integer") {
		t.Fatalf("expected actionable uploadthing timeout error, got: %v", err)
	}
}

func TestLoadFromEnvTrimsUploadLocalDir(t *testing.T) {
	uploadDir := filepath.Join(t.TempDir(), "uploads")
	setValidWebEnv(t, uploadDir)
	t.Setenv("UPLOAD_LOCAL_DIR", "  "+uploadDir+"  ")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.Storage.LocalDir != uploadDir {
		t.Fatalf("expected trimmed local dir %q, got %q", uploadDir, cfg.Storage.LocalDir)
	}
}

func TestLoadFromEnvRejectsLocalModeWithoutLocalDir(t *testing.T) {
	setValidWebEnv(t, filepath.Join(t.TempDir(), "uploads"))
	t.Setenv("UPLOAD_LOCAL_DIR", "")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error when local mode has empty UPLOAD_LOCAL_DIR")
	}

	if !strings.Contains(err.Error(), "UPLOAD_LOCAL_DIR is required when UPLOAD_STORAGE_MODE=local") {
		t.Fatalf("expected actionable local dir error, got: %v", err)
	}
}

func TestLoadFromEnvRejectsNonCreatableLocalDir(t *testing.T) {
	blockingFile := filepath.Join(t.TempDir(), "uploads-file")
	if err := os.WriteFile(blockingFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("failed to create blocking file: %v", err)
	}
	setValidWebEnv(t, blockingFile)

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error when UPLOAD_LOCAL_DIR points to non-directory path")
	}

	if !strings.Contains(err.Error(), "UPLOAD_LOCAL_DIR must be creatable") {
		t.Fatalf("expected actionable non-creatable dir error, got: %v", err)
	}
}

func TestLoadFromEnvRejectsNonWritableLocalDir(t *testing.T) {
	uploadDir := filepath.Join(t.TempDir(), "uploads")
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		t.Fatalf("failed to create upload directory: %v", err)
	}
	if err := os.Chmod(uploadDir, 0o555); err != nil {
		t.Fatalf("failed to make upload directory non-writable: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(uploadDir, 0o755)
	})

	probeFile := filepath.Join(uploadDir, "permission-probe")
	if err := os.WriteFile(probeFile, []byte("x"), 0o644); err == nil {
		_ = os.Remove(probeFile)
		t.Skip("cannot simulate non-writable upload directory in this environment")
	}

	setValidWebEnv(t, uploadDir)
	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error when UPLOAD_LOCAL_DIR is non-writable")
	}

	if !strings.Contains(err.Error(), "UPLOAD_LOCAL_DIR must be writable") {
		t.Fatalf("expected actionable non-writable dir error, got: %v", err)
	}
}

func TestLoadFromEnvSetsDefaultCORSOrigins(t *testing.T) {
	uploadDir := filepath.Join(t.TempDir(), "uploads")
	setValidWebEnv(t, uploadDir)
	t.Setenv("CORS_ALLOWED_ORIGINS", "")
	t.Setenv("FRONTEND_PORT", "4173")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	expectedOrigins := []string{
		"http://127.0.0.1:4173",
		"http://localhost:4173",
		"http://127.0.0.1:5173",
		"http://localhost:5173",
		"http://192.168.0.39:4173",
	}

	if len(cfg.CORSOrigins) != len(expectedOrigins) {
		t.Fatalf("expected %d CORS origins, got %d (%v)", len(expectedOrigins), len(cfg.CORSOrigins), cfg.CORSOrigins)
	}
	for idx, expected := range expectedOrigins {
		if cfg.CORSOrigins[idx] != expected {
			t.Fatalf("expected CORS origin %q at index %d, got %q", expected, idx, cfg.CORSOrigins[idx])
		}
	}
}

func TestLoadFromEnvUsesExplicitCORSOrigins(t *testing.T) {
	uploadDir := filepath.Join(t.TempDir(), "uploads")
	setValidWebEnv(t, uploadDir)
	t.Setenv("CORS_ALLOWED_ORIGINS", " https://app.example.com , http://localhost:3000,https://app.example.com ")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	expectedOrigins := []string{
		"https://app.example.com",
		"http://localhost:3000",
	}

	if len(cfg.CORSOrigins) != len(expectedOrigins) {
		t.Fatalf("expected %d CORS origins, got %d (%v)", len(expectedOrigins), len(cfg.CORSOrigins), cfg.CORSOrigins)
	}
	for idx, expected := range expectedOrigins {
		if cfg.CORSOrigins[idx] != expected {
			t.Fatalf("expected CORS origin %q at index %d, got %q", expected, idx, cfg.CORSOrigins[idx])
		}
	}
}

func TestLoadFromEnvLoadsBetterstackSettings(t *testing.T) {
	uploadDir := filepath.Join(t.TempDir(), "uploads")
	setValidWebEnv(t, uploadDir)
	t.Setenv("BETTERSTACK_SOURCE_TOKEN", "source-token")
	t.Setenv("BETTERSTACK_INGESTING_HOST", "https://in.logs.betterstack.com")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.BetterstackToken != "source-token" {
		t.Fatalf("expected betterstack token to be loaded, got %q", cfg.BetterstackToken)
	}
	if cfg.BetterstackEndpoint != "https://in.logs.betterstack.com/" {
		t.Fatalf("expected betterstack endpoint to be loaded, got %q", cfg.BetterstackEndpoint)
	}
}

func TestLoadFromEnvNormalizesBetterstackHostToHTTPSURL(t *testing.T) {
	uploadDir := filepath.Join(t.TempDir(), "uploads")
	setValidWebEnv(t, uploadDir)
	t.Setenv("BETTERSTACK_SOURCE_TOKEN", "source-token")
	t.Setenv("BETTERSTACK_INGESTING_HOST", "endpoint.betterstack.com")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.BetterstackEndpoint != "https://endpoint.betterstack.com/" {
		t.Fatalf("expected normalized betterstack endpoint, got %q", cfg.BetterstackEndpoint)
	}
}

func setValidWebEnv(t *testing.T, uploadDir string) {
	t.Helper()
	t.Setenv("APP_ENV", "local")
	t.Setenv("WEB_PORT", "8080")
	t.Setenv("DATABASE_PATH", "./var/app.db")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DATABASE_AUTH_TOKEN", "")
	t.Setenv("UPLOAD_STORAGE_MODE", UploadStorageModeLocal)
	t.Setenv("UPLOAD_LOCAL_DIR", uploadDir)
	t.Setenv("UPLOADTHING_TOKEN", "")
	t.Setenv("UPLOADTHING_PREPARE_UPLOAD_URL", "")
	t.Setenv("UPLOADTHING_REQUEST_TIMEOUT_SECS", "")
	t.Setenv("BETTERSTACK_SOURCE_TOKEN", "")
	t.Setenv("BETTERSTACK_INGESTING_HOST", "")
}
