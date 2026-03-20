package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFromEnvLocalModeCreatesWritableUploadDir(t *testing.T) {
	uploadDir := filepath.Join(t.TempDir(), "testdata", "uploads")
	setValidWorkerEnv(t, uploadDir)

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
	if cfg.GameMasterPath == "" {
		t.Fatal("expected WORKER_GAMEMASTER_PATH to be loaded")
	}
	if cfg.VideoSamplingIntervalMS != 300 {
		t.Fatalf("expected default VideoSamplingIntervalMS=300, got %d", cfg.VideoSamplingIntervalMS)
	}

	if _, err := os.Stat(uploadDir); err != nil {
		t.Fatalf("expected upload directory to exist: %v", err)
	}

	probeFile := filepath.Join(uploadDir, "probe.txt")
	if err := os.WriteFile(probeFile, []byte("ok"), 0o644); err != nil {
		t.Fatalf("expected upload directory to be writable: %v", err)
	}
}

func TestLoadFromEnvUsesVideoSamplingIntervalOverride(t *testing.T) {
	setValidWorkerEnv(t, filepath.Join(t.TempDir(), "uploads"))
	t.Setenv("WORKER_VIDEO_SAMPLING_INTERVAL_MS", "450")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.VideoSamplingIntervalMS != 450 {
		t.Fatalf("expected video sampling interval 450, got %d", cfg.VideoSamplingIntervalMS)
	}
}

func TestLoadFromEnvRejectsInvalidVideoSamplingInterval(t *testing.T) {
	setValidWorkerEnv(t, filepath.Join(t.TempDir(), "uploads"))
	t.Setenv("WORKER_VIDEO_SAMPLING_INTERVAL_MS", "0")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error for invalid WORKER_VIDEO_SAMPLING_INTERVAL_MS")
	}

	if !strings.Contains(err.Error(), "WORKER_VIDEO_SAMPLING_INTERVAL_MS must be a positive integer") {
		t.Fatalf("expected actionable video interval error, got: %v", err)
	}
}

func TestLoadFromEnvFailsWhenRequiredVarMissing(t *testing.T) {
	setValidWorkerEnv(t, filepath.Join(t.TempDir(), "uploads"))
	t.Setenv("APP_ENV", "")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error for missing APP_ENV")
	}

	if !strings.Contains(err.Error(), "APP_ENV is required") {
		t.Fatalf("expected actionable error for APP_ENV, got: %v", err)
	}
}

func TestLoadFromEnvRejectsInvalidPollInterval(t *testing.T) {
	setValidWorkerEnv(t, filepath.Join(t.TempDir(), "uploads"))
	t.Setenv("WORKER_POLL_INTERVAL_SECS", "0")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error for invalid WORKER_POLL_INTERVAL_SECS")
	}

	if !strings.Contains(err.Error(), "WORKER_POLL_INTERVAL_SECS must be a positive integer") {
		t.Fatalf("expected actionable poll interval error, got: %v", err)
	}
}

func TestLoadFromEnvUsesExplicitRemoteDatabaseURL(t *testing.T) {
	setValidWorkerEnv(t, filepath.Join(t.TempDir(), "unused"))
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_PATH", "")
	t.Setenv("DATABASE_URL", "libsql://example-org.turso.io")
	t.Setenv("DATABASE_AUTH_TOKEN", "secret-token")
	t.Setenv("UPLOAD_STORAGE_MODE", UploadStorageModeUploadThing)
	t.Setenv("UPLOAD_LOCAL_DIR", "")

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
	setValidWorkerEnv(t, filepath.Join(t.TempDir(), "unused"))
	t.Setenv("APP_ENV", "staging")
	t.Setenv("DATABASE_PATH", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("UPLOAD_STORAGE_MODE", UploadStorageModeUploadThing)
	t.Setenv("UPLOAD_LOCAL_DIR", "")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error when APP_ENV is not local and DATABASE_URL is missing")
	}

	if !strings.Contains(err.Error(), "DATABASE_URL is required when APP_ENV is not local") {
		t.Fatalf("expected actionable DATABASE_URL error, got: %v", err)
	}
}

func TestLoadFromEnvRejectsRemoteDatabaseWithoutAuthToken(t *testing.T) {
	setValidWorkerEnv(t, filepath.Join(t.TempDir(), "unused"))
	t.Setenv("APP_ENV", "staging")
	t.Setenv("DATABASE_PATH", "")
	t.Setenv("DATABASE_URL", "libsql://example-org.turso.io")
	t.Setenv("DATABASE_AUTH_TOKEN", "")
	t.Setenv("UPLOAD_STORAGE_MODE", UploadStorageModeUploadThing)
	t.Setenv("UPLOAD_LOCAL_DIR", "")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error when remote DATABASE_URL is set without DATABASE_AUTH_TOKEN")
	}

	if !strings.Contains(err.Error(), "DATABASE_AUTH_TOKEN is required for remote DATABASE_URL") {
		t.Fatalf("expected actionable DATABASE_AUTH_TOKEN error, got: %v", err)
	}
}

func TestLoadFromEnvRejectsAuthTokenForLocalDatabase(t *testing.T) {
	setValidWorkerEnv(t, filepath.Join(t.TempDir(), "uploads"))
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

func TestLoadFromEnvRejectsMissingGameMasterPath(t *testing.T) {
	setValidWorkerEnv(t, filepath.Join(t.TempDir(), "uploads"))
	t.Setenv("WORKER_GAMEMASTER_PATH", "")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error for missing WORKER_GAMEMASTER_PATH")
	}

	if !strings.Contains(err.Error(), "WORKER_GAMEMASTER_PATH is required") {
		t.Fatalf("expected actionable WORKER_GAMEMASTER_PATH error, got: %v", err)
	}
}

func TestLoadFromEnvRejectsNonexistentGameMasterPath(t *testing.T) {
	setValidWorkerEnv(t, filepath.Join(t.TempDir(), "uploads"))
	t.Setenv("WORKER_GAMEMASTER_PATH", filepath.Join(t.TempDir(), "missing-gamemaster.json"))

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error for missing WORKER_GAMEMASTER_PATH file")
	}

	if !strings.Contains(err.Error(), "WORKER_GAMEMASTER_PATH must point to an existing file") {
		t.Fatalf("expected actionable WORKER_GAMEMASTER_PATH file error, got: %v", err)
	}
}

func TestLoadFromEnvAcceptsUploadThingWithoutLocalDir(t *testing.T) {
	setValidWorkerEnv(t, filepath.Join(t.TempDir(), "unused"))
	t.Setenv("UPLOAD_STORAGE_MODE", UploadStorageModeUploadThing)
	t.Setenv("UPLOAD_LOCAL_DIR", "")

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
	if cfg.Storage.UploadThingDownloadTimeoutSecs != 30 {
		t.Fatalf("expected default uploadthing download timeout 30s, got %d", cfg.Storage.UploadThingDownloadTimeoutSecs)
	}
	if cfg.Storage.UploadThingDownloadRetryCount != 2 {
		t.Fatalf("expected default uploadthing retry count 2, got %d", cfg.Storage.UploadThingDownloadRetryCount)
	}
	if cfg.Storage.UploadThingDownloadTempDir != "" {
		t.Fatalf("expected empty uploadthing temp dir by default, got %q", cfg.Storage.UploadThingDownloadTempDir)
	}
}

func TestLoadFromEnvRejectsUnsupportedStorageMode(t *testing.T) {
	setValidWorkerEnv(t, filepath.Join(t.TempDir(), "uploads"))
	t.Setenv("UPLOAD_STORAGE_MODE", "s3")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error for unsupported UPLOAD_STORAGE_MODE")
	}

	if !strings.Contains(err.Error(), "UPLOAD_STORAGE_MODE must be one of") {
		t.Fatalf("expected actionable storage mode error, got: %v", err)
	}
}

func TestLoadFromEnvRejectsInvalidUploadThingDownloadTimeout(t *testing.T) {
	setValidWorkerEnv(t, filepath.Join(t.TempDir(), "unused"))
	t.Setenv("UPLOAD_STORAGE_MODE", UploadStorageModeUploadThing)
	t.Setenv("UPLOAD_LOCAL_DIR", "")
	t.Setenv("WORKER_UPLOADTHING_DOWNLOAD_TIMEOUT_SECS", "0")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error for invalid WORKER_UPLOADTHING_DOWNLOAD_TIMEOUT_SECS")
	}

	if !strings.Contains(err.Error(), "WORKER_UPLOADTHING_DOWNLOAD_TIMEOUT_SECS must be a positive integer") {
		t.Fatalf("expected actionable uploadthing timeout error, got: %v", err)
	}
}

func TestLoadFromEnvRejectsInvalidUploadThingDownloadRetryCount(t *testing.T) {
	setValidWorkerEnv(t, filepath.Join(t.TempDir(), "unused"))
	t.Setenv("UPLOAD_STORAGE_MODE", UploadStorageModeUploadThing)
	t.Setenv("UPLOAD_LOCAL_DIR", "")
	t.Setenv("WORKER_UPLOADTHING_DOWNLOAD_RETRY_COUNT", "-1")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error for invalid WORKER_UPLOADTHING_DOWNLOAD_RETRY_COUNT")
	}

	if !strings.Contains(err.Error(), "WORKER_UPLOADTHING_DOWNLOAD_RETRY_COUNT must be zero or greater") {
		t.Fatalf("expected actionable uploadthing retry error, got: %v", err)
	}
}

func TestLoadFromEnvUploadThingModeAcceptsWritableTempDir(t *testing.T) {
	setValidWorkerEnv(t, filepath.Join(t.TempDir(), "unused"))
	tempDir := filepath.Join(t.TempDir(), "download-tmp")
	t.Setenv("UPLOAD_STORAGE_MODE", UploadStorageModeUploadThing)
	t.Setenv("UPLOAD_LOCAL_DIR", "")
	t.Setenv("WORKER_UPLOADTHING_DOWNLOAD_TEMP_DIR", tempDir)

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("expected uploadthing mode with temp dir to load, got: %v", err)
	}
	if cfg.Storage.UploadThingDownloadTempDir != tempDir {
		t.Fatalf("expected uploadthing temp dir %q, got %q", tempDir, cfg.Storage.UploadThingDownloadTempDir)
	}
}

func TestLoadFromEnvRejectsNonCreatableUploadThingTempDir(t *testing.T) {
	setValidWorkerEnv(t, filepath.Join(t.TempDir(), "unused"))
	blockingFile := filepath.Join(t.TempDir(), "uploadthing-temp-file")
	if err := os.WriteFile(blockingFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("failed to create blocking file: %v", err)
	}

	t.Setenv("UPLOAD_STORAGE_MODE", UploadStorageModeUploadThing)
	t.Setenv("UPLOAD_LOCAL_DIR", "")
	t.Setenv("WORKER_UPLOADTHING_DOWNLOAD_TEMP_DIR", blockingFile)

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error when uploadthing temp dir points to non-directory path")
	}

	if !strings.Contains(err.Error(), "WORKER_UPLOADTHING_DOWNLOAD_TEMP_DIR must be creatable") {
		t.Fatalf("expected actionable uploadthing temp dir error, got: %v", err)
	}
}

func TestLoadFromEnvTrimsUploadLocalDir(t *testing.T) {
	uploadDir := filepath.Join(t.TempDir(), "uploads")
	setValidWorkerEnv(t, uploadDir)
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
	setValidWorkerEnv(t, filepath.Join(t.TempDir(), "uploads"))
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
	setValidWorkerEnv(t, blockingFile)

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

	setValidWorkerEnv(t, uploadDir)
	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error when UPLOAD_LOCAL_DIR is non-writable")
	}

	if !strings.Contains(err.Error(), "UPLOAD_LOCAL_DIR must be writable") {
		t.Fatalf("expected actionable non-writable dir error, got: %v", err)
	}
}

func TestLoadFromEnvLoadsBetterstackSettings(t *testing.T) {
	setValidWorkerEnv(t, filepath.Join(t.TempDir(), "uploads"))
	t.Setenv("BETTERSTACK_SOURCE_TOKEN", "source-token")
	t.Setenv("BETTERSTACK_INGESTING_HOST", "https://in.logs.betterstack.com")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.BetterstackToken != "source-token" {
		t.Fatalf("expected betterstack token to be loaded, got %q", cfg.BetterstackToken)
	}
	if cfg.BetterstackEndpoint != "https://in.logs.betterstack.com" {
		t.Fatalf("expected betterstack endpoint to be loaded, got %q", cfg.BetterstackEndpoint)
	}
}

func setValidWorkerEnv(t *testing.T, uploadDir string) {
	t.Helper()

	gameMasterPath := filepath.Join(filepath.Dir(uploadDir), "gamemaster.json")
	if err := os.MkdirAll(filepath.Dir(gameMasterPath), 0o755); err != nil {
		t.Fatalf("failed to create gamemaster directory: %v", err)
	}
	gameMasterContent := `{
  "munna": {
    "speciesName": "Munna",
    "speciesId": "munna",
    "baseStats": { "atk": 98, "def": 138, "hp": 183 }
  }
}`
	if err := os.WriteFile(gameMasterPath, []byte(gameMasterContent), 0o644); err != nil {
		t.Fatalf("failed to write gamemaster fixture: %v", err)
	}

	t.Setenv("APP_ENV", "local")
	t.Setenv("WORKER_POLL_INTERVAL_SECS", "2")
	t.Setenv("WORKER_HEALTH_PORT", "8081")
	t.Setenv("DATABASE_PATH", "./var/app.db")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DATABASE_AUTH_TOKEN", "")
	t.Setenv("WORKER_GAMEMASTER_PATH", gameMasterPath)
	t.Setenv("UPLOAD_STORAGE_MODE", UploadStorageModeLocal)
	t.Setenv("UPLOAD_LOCAL_DIR", uploadDir)
	t.Setenv("WORKER_UPLOADTHING_DOWNLOAD_TIMEOUT_SECS", "")
	t.Setenv("WORKER_UPLOADTHING_DOWNLOAD_RETRY_COUNT", "")
	t.Setenv("WORKER_UPLOADTHING_DOWNLOAD_TEMP_DIR", "")
	t.Setenv("BETTERSTACK_SOURCE_TOKEN", "")
	t.Setenv("BETTERSTACK_INGESTING_HOST", "")
}
