package worker

import (
	"fmt"
	"os"
	"time"
)

func newWorkerID() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown-host"
	}

	return fmt.Sprintf("%s-%d", hostname, os.Getpid())
}

func leaseTimeoutForPollInterval(pollIntervalSecs int) time.Duration {
	interval := time.Duration(pollIntervalSecs) * time.Second
	timeout := 3 * interval
	const minTimeout = 30 * time.Second
	if timeout < minTimeout {
		return minTimeout
	}

	return timeout
}

func heartbeatIntervalForPollInterval(pollIntervalSecs int) time.Duration {
	interval := time.Duration(pollIntervalSecs) * time.Second
	heartbeat := interval / 2
	if heartbeat < time.Second {
		return time.Second
	}

	return heartbeat
}
