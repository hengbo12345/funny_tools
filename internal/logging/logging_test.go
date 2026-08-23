package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestMaskSensitiveString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "GET /config/secret-token-12345 HTTP/1.1",
			expected: "GET /config/*** HTTP/1.1",
		},
		{
			input:    "Authorization: Bearer secret-auth-token-999",
			expected: "Authorization: ***",
		},
		{
			input:    "Cookie: session_id=abcdef123456",
			expected: "Cookie: ***",
		},
		{
			input:    "https://example.com/sub?token=mysecret&flag=1",
			expected: "https://example.com/sub?token=***&flag=1",
		},
		{
			input:    "Normal log message without secrets",
			expected: "Normal log message without secrets",
		},
	}

	for _, tt := range tests {
		got := MaskSensitiveString(tt.input)
		if got != tt.expected {
			t.Errorf("MaskSensitiveString(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestMaskingHandler(t *testing.T) {
	var buf bytes.Buffer
	logger := Init(&buf, slog.LevelInfo, true)

	logger.Info("fetching source",
		slog.String("url", "https://example.com/config/token-abc-123"),
		slog.String("token", "raw-secret-value"),
		slog.String("user", "alice"),
	)

	out := buf.String()
	if strings.Contains(out, "token-abc-123") {
		t.Errorf("expected token in URL to be masked, got: %s", out)
	}
	if strings.Contains(out, "raw-secret-value") {
		t.Errorf("expected secret value in token attr to be masked, got: %s", out)
	}
	if !strings.Contains(out, "alice") {
		t.Errorf("expected non-sensitive attr to be preserved, got: %s", out)
	}
}
