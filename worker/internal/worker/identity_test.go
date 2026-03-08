package worker

import (
	"strings"
	"testing"
	"time"
)

func TestLeaseTimeoutForPollInterval(t *testing.T) {
	if got := leaseTimeoutForPollInterval(2); got != 30*time.Second {
		t.Fatalf("expected 30s minimum timeout, got %s", got)
	}

	if got := leaseTimeoutForPollInterval(15); got != 45*time.Second {
		t.Fatalf("expected 45s timeout, got %s", got)
	}
}

func TestWorkerIDIncludesProcessID(t *testing.T) {
	workerID := newWorkerID()
	if workerID == "" {
		t.Fatal("expected non-empty worker ID")
	}

	if !strings.Contains(workerID, "-") {
		t.Fatalf("expected worker ID with hostname-pid format, got %q", workerID)
	}
}
