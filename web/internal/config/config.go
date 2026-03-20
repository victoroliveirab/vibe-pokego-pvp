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

	defaultUploadThingPrepareUploadURL   = "https://api.uploadthing.com/v7/prepareUpload"
	defaultUploadThingRequestTimeoutSecs = 30
)

// StorageConfig holds upload storage runtime settings.
type StorageConfig struct {
	Mode                          string
	LocalDir                      string
	UploadThingToken              string
	UploadThingPrepareUploadURL   string
	UploadThingRequestTimeoutSecs int
}

// Config holds web runtime settings.
type Config struct {
	AppEnv              string
	Port                int
	DatabasePath        string
	DatabaseURL         string
	DatabaseAuthToken   string
	DatabaseIsLocal     bool
	BetterstackToken    string
	BetterstackEndpoint string
	Storage             StorageConfig
	CORSOrigins         []string
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

// MustLoadFromEnv loads required runtime settings and exits fast on invalid input.
func MustLoadFromEnv() Config {
	cfg, err := LoadFromEnv()
	if err != nil {
		panic(err)
	}
	return cfg
}

// LoadFromEnv parses web settings from environment variables.
func LoadFromEnv() (Config, error) {
	appEnv, err := requiredEnv("APP_ENV")
	if err != nil {
		return Config{}, err
	}

	port, err := requiredPositiveInt("WEB_PORT")
	if err != nil {
		return Config{}, err
	}

	databaseConfig, err := loadDatabaseConfig(appEnv)
	if err != nil {
		return Config{}, err
	}

	storageMode, err := requiredEnv("UPLOAD_STORAGE_MODE")
	if err != nil {
		return Config{}, err
	}
	uploadThingRequestTimeoutSecs, err := optionalPositiveInt(
		"UPLOADTHING_REQUEST_TIMEOUT_SECS",
		defaultUploadThingRequestTimeoutSecs,
	)
	if err != nil {
		return Config{}, err
	}
	uploadThingPrepareUploadURL, err := normalizeUploadThingPrepareUploadURL(
		os.Getenv("UPLOADTHING_PREPARE_UPLOAD_URL"),
	)
	if err != nil {
		return Config{}, err
	}

	storage := StorageConfig{
		Mode:                          storageMode,
		LocalDir:                      strings.TrimSpace(os.Getenv("UPLOAD_LOCAL_DIR")),
		UploadThingToken:              strings.TrimSpace(os.Getenv("UPLOADTHING_TOKEN")),
		UploadThingPrepareUploadURL:   uploadThingPrepareUploadURL,
		UploadThingRequestTimeoutSecs: uploadThingRequestTimeoutSecs,
	}
	if err := validateStorageConfig(storage); err != nil {
		return Config{}, err
	}

	return Config{
		AppEnv:              appEnv,
		Port:                port,
		DatabasePath:        databaseConfig.Path,
		DatabaseURL:         databaseConfig.URL,
		DatabaseAuthToken:   databaseConfig.AuthToken,
		DatabaseIsLocal:     databaseConfig.IsLocal,
		BetterstackToken:    strings.TrimSpace(os.Getenv("BETTERSTACK_SOURCE_TOKEN")),
		BetterstackEndpoint: strings.TrimSpace(os.Getenv("BETTERSTACK_INGESTING_HOST")),
		Storage:             storage,
		CORSOrigins:         parseCORSOrigins(),
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

func parseCORSOrigins() []string {
	raw := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS"))
	if raw == "" {
		return defaultCORSOrigins()
	}

	parts := strings.Split(raw, ",")
	return dedupeNonEmpty(parts)
}

func defaultCORSOrigins() []string {
	frontendPort := strings.TrimSpace(os.Getenv("FRONTEND_PORT"))
	if frontendPort == "" {
		frontendPort = "4173"
	}

	return dedupeNonEmpty([]string{
		fmt.Sprintf("http://127.0.0.1:%s", frontendPort),
		fmt.Sprintf("http://localhost:%s", frontendPort),
		"http://127.0.0.1:5173",
		"http://localhost:5173",
		"http://192.168.0.39:4173",
	})
}

func dedupeNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))

	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}

	return result
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
		if storage.UploadThingToken == "" {
			return fmt.Errorf("UPLOADTHING_TOKEN is required when UPLOAD_STORAGE_MODE=uploadthing")
		}
		if storage.UploadThingRequestTimeoutSecs <= 0 {
			return fmt.Errorf("UPLOADTHING_REQUEST_TIMEOUT_SECS must be a positive integer")
		}
		if _, err := normalizeUploadThingPrepareUploadURL(storage.UploadThingPrepareUploadURL); err != nil {
			return err
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

func normalizeUploadThingPrepareUploadURL(value string) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		normalized = defaultUploadThingPrepareUploadURL
	}

	parsed, err := url.Parse(normalized)
	if err != nil {
		return "", fmt.Errorf("UPLOADTHING_PREPARE_UPLOAD_URL must be a valid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("UPLOADTHING_PREPARE_UPLOAD_URL must use http or https scheme")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("UPLOADTHING_PREPARE_UPLOAD_URL must include a host")
	}

	return parsed.String(), nil
}

func ensureWritableDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("UPLOAD_LOCAL_DIR must be creatable: %w", err)
	}

	absPath, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("UPLOAD_LOCAL_DIR could not be resolved: %w", err)
	}

	testFile, err := os.CreateTemp(absPath, ".writecheck-*")
	if err != nil {
		return fmt.Errorf("UPLOAD_LOCAL_DIR must be writable: %w", err)
	}

	testFileName := testFile.Name()
	if closeErr := testFile.Close(); closeErr != nil {
		return fmt.Errorf("UPLOAD_LOCAL_DIR write check failed: %w", closeErr)
	}

	if removeErr := os.Remove(testFileName); removeErr != nil {
		return fmt.Errorf("UPLOAD_LOCAL_DIR cleanup failed: %w", removeErr)
	}

	return nil
}
