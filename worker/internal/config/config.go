package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	UploadStorageModeLocal       = "local"
	UploadStorageModeUploadThing = "uploadthing"

	defaultUploadThingDownloadTimeoutSecs = 30
	defaultUploadThingDownloadRetryCount  = 2
)

// StorageConfig holds upload storage runtime settings.
type StorageConfig struct {
	Mode                           string
	LocalDir                       string
	UploadThingDownloadTimeoutSecs int
	UploadThingDownloadRetryCount  int
	UploadThingDownloadTempDir     string
}

// Config holds worker runtime settings.
type Config struct {
	AppEnv                  string
	PollIntervalSecs        int
	HealthPort              int
	DatabasePath            string
	DatabaseURL             string
	DatabaseAuthToken       string
	DatabaseIsLocal         bool
	BetterstackToken        string
	BetterstackEndpoint     string
	GameMasterPath          string
	VideoSamplingIntervalMS int
	Storage                 StorageConfig
}

// DatabaseDSN returns a libsql driver DSN with auth token attached for remote connections.
func (c Config) DatabaseDSN() string {
	dsn := strings.TrimSpace(c.DatabaseURL)
	if dsn == "" {
		if strings.TrimSpace(c.DatabasePath) == "" {
			return ""
		}
		dsn = formatLocalDatabaseURL(c.DatabasePath)
	}
	if c.DatabaseIsLocal {
		return dsn
	}

	authToken := strings.TrimSpace(c.DatabaseAuthToken)
	if authToken == "" {
		return dsn
	}

	parsed, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}

	query := parsed.Query()
	if strings.TrimSpace(query.Get("authToken")) == "" {
		query.Set("authToken", authToken)
		parsed.RawQuery = query.Encode()
	}

	return parsed.String()
}

// MustLoadFromEnv loads required worker settings and exits fast on invalid input.
func MustLoadFromEnv() Config {
	cfg, err := LoadFromEnv()
	if err != nil {
		panic(err)
	}
	return cfg
}

// LoadFromEnv parses worker settings from environment variables.
func LoadFromEnv() (Config, error) {
	appEnv, err := requiredEnv("APP_ENV")
	if err != nil {
		return Config{}, err
	}

	pollSecs, err := requiredPositiveInt("WORKER_POLL_INTERVAL_SECS")
	if err != nil {
		return Config{}, err
	}

	healthPort, err := requiredPositiveInt("WORKER_HEALTH_PORT")
	if err != nil {
		return Config{}, err
	}

	databaseConfig, err := loadDatabaseConfig(appEnv)
	if err != nil {
		return Config{}, err
	}

	gameMasterPath, err := requiredEnv("WORKER_GAMEMASTER_PATH")
	if err != nil {
		return Config{}, err
	}
	if err := ensureReadableFile("WORKER_GAMEMASTER_PATH", gameMasterPath); err != nil {
		return Config{}, err
	}

	videoSamplingIntervalMS, err := optionalPositiveInt("WORKER_VIDEO_SAMPLING_INTERVAL_MS", 300)
	if err != nil {
		return Config{}, err
	}

	storageMode, err := requiredEnv("UPLOAD_STORAGE_MODE")
	if err != nil {
		return Config{}, err
	}
	uploadThingDownloadTimeoutSecs, err := optionalPositiveInt(
		"WORKER_UPLOADTHING_DOWNLOAD_TIMEOUT_SECS",
		defaultUploadThingDownloadTimeoutSecs,
	)
	if err != nil {
		return Config{}, err
	}
	uploadThingDownloadRetryCount, err := optionalNonNegativeInt(
		"WORKER_UPLOADTHING_DOWNLOAD_RETRY_COUNT",
		defaultUploadThingDownloadRetryCount,
	)
	if err != nil {
		return Config{}, err
	}

	storage := StorageConfig{
		Mode:                           storageMode,
		LocalDir:                       strings.TrimSpace(os.Getenv("UPLOAD_LOCAL_DIR")),
		UploadThingDownloadTimeoutSecs: uploadThingDownloadTimeoutSecs,
		UploadThingDownloadRetryCount:  uploadThingDownloadRetryCount,
		UploadThingDownloadTempDir:     strings.TrimSpace(os.Getenv("WORKER_UPLOADTHING_DOWNLOAD_TEMP_DIR")),
	}
	if err := validateStorageConfig(storage); err != nil {
		return Config{}, err
	}

	betterstackEndpoint, err := normalizeBetterstackEndpoint(os.Getenv("BETTERSTACK_INGESTING_HOST"))
	if err != nil {
		return Config{}, err
	}

	return Config{
		AppEnv:                  appEnv,
		PollIntervalSecs:        pollSecs,
		HealthPort:              healthPort,
		DatabasePath:            databaseConfig.Path,
		DatabaseURL:             databaseConfig.URL,
		DatabaseAuthToken:       databaseConfig.AuthToken,
		DatabaseIsLocal:         databaseConfig.IsLocal,
		BetterstackToken:        strings.TrimSpace(os.Getenv("BETTERSTACK_SOURCE_TOKEN")),
		BetterstackEndpoint:     betterstackEndpoint,
		GameMasterPath:          gameMasterPath,
		VideoSamplingIntervalMS: videoSamplingIntervalMS,
		Storage:                 storage,
	}, nil
}

type resolvedDatabaseConfig struct {
	Path      string
	URL       string
	AuthToken string
	IsLocal   bool
}

func loadDatabaseConfig(appEnv string) (resolvedDatabaseConfig, error) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	databasePath := strings.TrimSpace(os.Getenv("DATABASE_PATH"))
	databaseAuthToken := strings.TrimSpace(os.Getenv("DATABASE_AUTH_TOKEN"))

	if databaseURL == "" {
		if appEnv != "local" {
			return resolvedDatabaseConfig{}, fmt.Errorf("DATABASE_URL is required when APP_ENV is not local")
		}
		if databasePath == "" {
			return resolvedDatabaseConfig{}, fmt.Errorf("DATABASE_PATH is required when DATABASE_URL is not set in local mode")
		}
		databaseURL = formatLocalDatabaseURL(databasePath)
	}

	isLocal := isLocalDatabaseURL(databaseURL)
	if isLocal {
		if databaseAuthToken != "" {
			return resolvedDatabaseConfig{}, fmt.Errorf("DATABASE_AUTH_TOKEN must be empty for local database connections")
		}
		if databasePath == "" {
			databasePath = parseLocalDatabasePath(databaseURL)
		}
		return resolvedDatabaseConfig{
			Path:    databasePath,
			URL:     databaseURL,
			IsLocal: true,
		}, nil
	}

	if databaseAuthToken == "" {
		return resolvedDatabaseConfig{}, fmt.Errorf("DATABASE_AUTH_TOKEN is required for remote DATABASE_URL")
	}

	return resolvedDatabaseConfig{
		Path:      databasePath,
		URL:       databaseURL,
		AuthToken: databaseAuthToken,
		IsLocal:   false,
	}, nil
}

func formatLocalDatabaseURL(databasePath string) string {
	trimmed := strings.TrimSpace(databasePath)
	if strings.HasPrefix(strings.ToLower(trimmed), "file:") {
		return trimmed
	}
	return "file:" + trimmed
}

func normalizeBetterstackEndpoint(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}

	candidate := trimmed
	if !strings.Contains(candidate, "://") {
		candidate = "https://" + candidate
	}

	parsed, err := url.Parse(candidate)
	if err != nil {
		return "", fmt.Errorf("BETTERSTACK_INGESTING_HOST must be a valid URL or host: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("BETTERSTACK_INGESTING_HOST must be a valid URL or host")
	}
	if parsed.Path == "" {
		parsed.Path = "/"
	}

	return parsed.String(), nil
}

func parseLocalDatabasePath(databaseURL string) string {
	trimmed := strings.TrimSpace(databaseURL)
	if strings.HasPrefix(strings.ToLower(trimmed), "file:") {
		return trimmed[len("file:"):]
	}
	return trimmed
}

func isLocalDatabaseURL(databaseURL string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(databaseURL))
	if trimmed == "" {
		return false
	}
	switch {
	case strings.HasPrefix(trimmed, "libsql://"),
		strings.HasPrefix(trimmed, "https://"),
		strings.HasPrefix(trimmed, "http://"),
		strings.HasPrefix(trimmed, "wss://"),
		strings.HasPrefix(trimmed, "ws://"):
		return false
	default:
		return true
	}
}

func validateStorageConfig(storage StorageConfig) error {
	switch storage.Mode {
	case UploadStorageModeLocal:
		if storage.LocalDir == "" {
			return fmt.Errorf("UPLOAD_LOCAL_DIR is required when UPLOAD_STORAGE_MODE=local")
		}
		if err := ensureWritableDir(storage.LocalDir); err != nil {
			return err
		}
		return nil
	case UploadStorageModeUploadThing:
		if storage.UploadThingDownloadTimeoutSecs <= 0 {
			return fmt.Errorf("WORKER_UPLOADTHING_DOWNLOAD_TIMEOUT_SECS must be a positive integer")
		}
		if storage.UploadThingDownloadRetryCount < 0 {
			return fmt.Errorf("WORKER_UPLOADTHING_DOWNLOAD_RETRY_COUNT must be zero or greater")
		}
		if storage.UploadThingDownloadTempDir != "" {
			if err := ensureWritableDirForEnv(storage.UploadThingDownloadTempDir, "WORKER_UPLOADTHING_DOWNLOAD_TEMP_DIR"); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("UPLOAD_STORAGE_MODE must be one of: %s, %s", UploadStorageModeLocal, UploadStorageModeUploadThing)
	}
}

func requiredEnv(key string) (string, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

func requiredPositiveInt(key string) (int, error) {
	value, err := requiredEnv(key)
	if err != nil {
		return 0, err
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}

	return parsed, nil
}

func optionalPositiveInt(key string, defaultValue int) (int, error) {
	if defaultValue <= 0 {
		return 0, fmt.Errorf("default value must be a positive integer")
	}

	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}

	return parsed, nil
}

func optionalNonNegativeInt(key string, defaultValue int) (int, error) {
	if defaultValue < 0 {
		return 0, fmt.Errorf("default value must be non-negative integer")
	}

	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("%s must be zero or greater", key)
	}

	return parsed, nil
}

func ensureWritableDir(dir string) error {
	return ensureWritableDirForEnv(dir, "UPLOAD_LOCAL_DIR")
}

func ensureWritableDirForEnv(dir string, envKey string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("%s must be creatable: %w", envKey, err)
	}

	absPath, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("%s could not be resolved: %w", envKey, err)
	}

	testFile, err := os.CreateTemp(absPath, ".writecheck-*")
	if err != nil {
		return fmt.Errorf("%s must be writable: %w", envKey, err)
	}

	testFileName := testFile.Name()
	if closeErr := testFile.Close(); closeErr != nil {
		return fmt.Errorf("%s write check failed: %w", envKey, closeErr)
	}

	if removeErr := os.Remove(testFileName); removeErr != nil {
		return fmt.Errorf("%s cleanup failed: %w", envKey, removeErr)
	}

	return nil
}

func ensureReadableFile(envKey string, filePath string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("%s must point to an existing file: %w", envKey, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s must point to a file, got directory", envKey)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("%s must be readable: %w", envKey, err)
	}
	defer file.Close()

	return nil
}
