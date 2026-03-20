package logging

import (
	"fmt"
	"strings"
	"testing"
)

func TestNewUsesJSONHandlerOutsideProduction(t *testing.T) {
	logger := New(Config{Env: "local"})

	handlerType := fmt.Sprintf("%T", logger.Handler())
	if !strings.Contains(handlerType, "JSONHandler") {
		t.Fatalf("expected dev logger to use JSON handler, got %s", handlerType)
	}
}

func TestNewUsesBetterstackHandlerInProduction(t *testing.T) {
	logger := New(Config{
		Env:                 "production",
		BetterstackToken:    "token",
		BetterstackEndpoint: "https://in.logs.betterstack.com/",
	})

	handlerType := fmt.Sprintf("%T", logger.Handler())
	if !strings.Contains(strings.ToLower(handlerType), "betterstack") {
		t.Fatalf("expected production logger to use Betterstack handler, got %s", handlerType)
	}
}
