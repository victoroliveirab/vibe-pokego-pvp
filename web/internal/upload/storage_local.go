package upload

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const localMediaURLPrefix = "local://"

var errDestinationFileExists = errors.New("destination file already exists")

// StoredMedia describes persisted media in local mode.
type StoredMedia struct {
	MediaURL string
	FilePath string
}

// LocalMediaStorage persists validated media files under UPLOAD_LOCAL_DIR.
type LocalMediaStorage struct {
	localDir string
}

// NewLocalMediaStorage builds a local media writer rooted at localDir.
func NewLocalMediaStorage(localDir string) (*LocalMediaStorage, error) {
	trimmedDir := strings.TrimSpace(localDir)
	if trimmedDir == "" {
		return nil, fmt.Errorf("local media directory is required")
	}

	return &LocalMediaStorage{
		localDir: trimmedDir,
	}, nil
}

// StoreValidated persists a validated temp file in local storage.
// It returns a cleanup function that can be used to remove newly-written files
// if downstream persistence fails after this method succeeds.
func (s *LocalMediaStorage) StoreValidated(
	ctx context.Context,
	media ValidatedMedia,
) (StoredMedia, func() error, error) {
	if err := ctx.Err(); err != nil {
		return StoredMedia{}, nil, err
	}
	if media.TempFilePath == "" {
		return StoredMedia{}, nil, fmt.Errorf("validated media temp file path is required")
	}
	defer media.Cleanup()

	if err := os.MkdirAll(s.localDir, 0o755); err != nil {
		return StoredMedia{}, nil, fmt.Errorf("ensure local media directory: %w", err)
	}

	filename, err := deterministicFilename(media)
	if err != nil {
		return StoredMedia{}, nil, err
	}

	destinationPath := filepath.Join(s.localDir, filename)

	created := false
	if err := copyFileExclusive(media.TempFilePath, destinationPath); err != nil {
		if !errors.Is(err, errDestinationFileExists) {
			return StoredMedia{}, nil, err
		}
	} else {
		created = true
	}

	stored := StoredMedia{
		MediaURL: localMediaURL(filename),
		FilePath: destinationPath,
	}

	return stored, cleanupStoredMedia(destinationPath, created), nil
}

func cleanupStoredMedia(filePath string, created bool) func() error {
	var once sync.Once
	var cleanupErr error

	return func() error {
		once.Do(func() {
			if !created {
				return
			}
			if err := os.Remove(filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
				cleanupErr = fmt.Errorf("remove stored media file: %w", err)
			}
		})
		return cleanupErr
	}
}

func localMediaURL(filename string) string {
	return localMediaURLPrefix + path.Join("uploads", filename)
}

func deterministicFilename(media ValidatedMedia) (string, error) {
	hash, err := hashFileSHA256(media.TempFilePath)
	if err != nil {
		return "", err
	}

	ext := chooseExtension(media)
	return fmt.Sprintf("%s-%s%s", media.Kind, hash[:24], ext), nil
}

func hashFileSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open media for hashing: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("hash media file: %w", err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func chooseExtension(media ValidatedMedia) string {
	if ext := sanitizeExtension(filepath.Ext(media.OriginalFilename)); ext != "" {
		return ext
	}

	extensions, err := mime.ExtensionsByType(media.ContentType)
	if err == nil && len(extensions) > 0 {
		sort.Slice(extensions, func(i, j int) bool {
			if len(extensions[i]) != len(extensions[j]) {
				return len(extensions[i]) < len(extensions[j])
			}
			return extensions[i] < extensions[j]
		})
		for _, ext := range extensions {
			if normalized := sanitizeExtension(ext); normalized != "" {
				return normalized
			}
		}
	}

	return ".bin"
}

func sanitizeExtension(ext string) string {
	normalized := strings.TrimSpace(strings.ToLower(ext))
	if normalized == "" {
		return ""
	}
	if !strings.HasPrefix(normalized, ".") {
		normalized = "." + normalized
	}
	if len(normalized) == 1 {
		return ""
	}

	for _, r := range normalized[1:] {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return ""
		}
	}

	return normalized
}

func copyFileExclusive(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open source media file: %w", err)
	}
	defer src.Close()

	dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return errDestinationFileExists
		}
		return fmt.Errorf("create destination media file: %w", err)
	}

	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		_ = os.Remove(dstPath)
		return fmt.Errorf("copy media file: %w", err)
	}

	if err := dst.Close(); err != nil {
		_ = os.Remove(dstPath)
		return fmt.Errorf("close destination media file: %w", err)
	}

	return nil
}
