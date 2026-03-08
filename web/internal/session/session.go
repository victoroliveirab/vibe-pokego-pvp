package session

import (
	"crypto/rand"
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidID = errors.New("invalid session id")
	ErrNotFound  = errors.New("session not found")
)

// Session represents an anonymous web session persisted in storage.
type Session struct {
	ID         string
	CreatedAt  time.Time
	LastSeenAt time.Time
}

// NewID creates a UUID v4 for anonymous sessions.
func NewID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}

	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80

	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		buf[0:4],
		buf[4:6],
		buf[6:8],
		buf[8:10],
		buf[10:16],
	), nil
}

// ValidateID validates that the provided string is a UUID v4.
func ValidateID(id string) error {
	if len(id) != 36 {
		return ErrInvalidID
	}

	for _, idx := range []int{8, 13, 18, 23} {
		if id[idx] != '-' {
			return ErrInvalidID
		}
	}

	for i := 0; i < len(id); i++ {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		if !isHex(id[i]) {
			return ErrInvalidID
		}
	}

	if id[14] != '4' {
		return ErrInvalidID
	}

	if !isUUIDVariant(id[19]) {
		return ErrInvalidID
	}

	return nil
}

func isHex(ch byte) bool {
	return (ch >= '0' && ch <= '9') ||
		(ch >= 'a' && ch <= 'f') ||
		(ch >= 'A' && ch <= 'F')
}

func isUUIDVariant(ch byte) bool {
	switch ch {
	case '8', '9', 'a', 'A', 'b', 'B':
		return true
	default:
		return false
	}
}
