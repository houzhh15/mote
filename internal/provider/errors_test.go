package provider

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsContextWindowExceeded_TypedError(t *testing.T) {
	err := &ProviderError{
		Code:    ErrCodeContextWindowExceeded,
		Message: "some message",
	}
	if !IsContextWindowExceeded(err) {
		t.Fatal("expected IsContextWindowExceeded to return true for typed error")
	}
}

func TestIsContextWindowExceeded_WrappedTypedError(t *testing.T) {
	inner := &ProviderError{
		Code:    ErrCodeContextWindowExceeded,
		Message: "inner",
	}
	err := fmt.Errorf("outer: %w", inner)
	if !IsContextWindowExceeded(err) {
		t.Fatal("expected IsContextWindowExceeded to return true for wrapped typed error")
	}
}

func TestIsContextWindowExceeded_KeywordFallback(t *testing.T) {
	keywords := []string{
		"context window exceeded",
		"context length exceeded",
		"maximum context length",
		"token limit exceeded",
		"too many tokens",
	}
	for _, kw := range keywords {
		err := errors.New("provider error: " + kw + " for this model")
		if !IsContextWindowExceeded(err) {
			t.Fatalf("expected IsContextWindowExceeded to return true for keyword %q", kw)
		}
	}
}

func TestIsContextWindowExceeded_NegativeCases(t *testing.T) {
	cases := []error{
		errors.New("invalid request"),
		errors.New("rate limit exceeded"),
		&ProviderError{Code: ErrCodeRateLimited, Message: "rate limited"},
		nil,
	}
	for _, err := range cases {
		if IsContextWindowExceeded(err) {
			t.Fatalf("expected IsContextWindowExceeded to return false for %v", err)
		}
	}
}
