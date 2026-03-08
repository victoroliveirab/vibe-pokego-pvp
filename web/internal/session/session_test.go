package session

import (
	"testing"
)

func TestNewIDReturnsValidUUIDv4(t *testing.T) {
	for i := 0; i < 100; i++ {
		id, err := NewID()
		if err != nil {
			t.Fatalf("expected id, got error: %v", err)
		}

		if err := ValidateID(id); err != nil {
			t.Fatalf("expected generated id %q to validate, got: %v", id, err)
		}
	}
}

func TestValidateID(t *testing.T) {
	testCases := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{
			name:    "valid lowercase uuid v4",
			id:      "12f9f169-d9ca-4ea3-91e0-18356a1e1477",
			wantErr: false,
		},
		{
			name:    "valid uppercase uuid v4",
			id:      "12F9F169-D9CA-4EA3-91E0-18356A1E1477",
			wantErr: false,
		},
		{
			name:    "empty",
			id:      "",
			wantErr: true,
		},
		{
			name:    "invalid length",
			id:      "12345",
			wantErr: true,
		},
		{
			name:    "missing hyphens",
			id:      "12f9f169d9ca4ea391e018356a1e1477",
			wantErr: true,
		},
		{
			name:    "wrong version",
			id:      "12f9f169-d9ca-1ea3-91e0-18356a1e1477",
			wantErr: true,
		},
		{
			name:    "wrong variant",
			id:      "12f9f169-d9ca-4ea3-71e0-18356a1e1477",
			wantErr: true,
		},
		{
			name:    "invalid character",
			id:      "12f9f169-d9ca-4ea3-91e0-18356a1e147g",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateID(tc.id)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}
