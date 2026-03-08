package upload

import "testing"

func TestJobStatusConstants(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   string
	}{
		{name: "queued", status: JobStatusQueued, want: "QUEUED"},
		{name: "pending-user-dedup", status: JobStatusPendingUserDedup, want: "PENDING_USER_DEDUP"},
	}

	for _, tc := range tests {
		if tc.status != tc.want {
			t.Fatalf("%s: expected %q, got %q", tc.name, tc.want, tc.status)
		}
	}
}
