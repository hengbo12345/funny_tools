package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
)

var (
	// defaultLogger is the global logger instance
	defaultLogger *slog.Logger
	loggerOnce    sync.Once

	// regex patterns for masking sensitive data
	urlTokenRegex    = regexp.MustCompile(`(/config/)([^/?#\s]+)`)
	authHeaderRegex  = regexp.MustCompile(`(?i)(authorization:\s*)([^\r\n,]+)`)
	cookieRegex      = regexp.MustCompile(`(?i)(cookie:\s*)([^\r\n,]+)`)
	passwordURIParam = regexp.MustCompile(`(?i)(password|secret|token|key)=([^&]+)`)
)

// Init initializes the global logger.
func Init(w io.Writer, level slog.Level, jsonFormat bool) *slog.Logger {
	if w == nil {
		w = os.Stderr
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	var baseHandler slog.Handler
	if jsonFormat {
		baseHandler = slog.NewJSONHandler(w, opts)
	} else {
		baseHandler = slog.NewTextHandler(w, opts)
	}

	maskedHandler := &MaskingHandler{Handler: baseHandler}
	logger := slog.New(maskedHandler)
	slog.SetDefault(logger)
	defaultLogger = logger
	return logger
}

// Logger returns the global logger instance.
func Logger() *slog.Logger {
	loggerOnce.Do(func() {
		if defaultLogger == nil {
			defaultLogger = Init(os.Stderr, slog.LevelInfo, true)
		}
	})
	return defaultLogger
}

// MaskingHandler is a slog.Handler middleware that masks sensitive strings in attributes.
type MaskingHandler struct {
	slog.Handler
}

func (h *MaskingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.Handler.Enabled(ctx, level)
}

func (h *MaskingHandler) Handle(ctx context.Context, r slog.Record) error {
	var maskedAttrs []slog.Attr
	r.Attrs(func(a slog.Attr) bool {
		maskedAttrs = append(maskedAttrs, sanitizeAttr(a))
		return true
	})

	// Create a new record with sanitized message and attributes
	sanitizedMsg := MaskSensitiveString(r.Message)
	newRecord := slog.NewRecord(r.Time, r.Level, sanitizedMsg, r.PC)
	newRecord.AddAttrs(maskedAttrs...)

	return h.Handler.Handle(ctx, newRecord)
}

func (h *MaskingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	sanitizedAttrs := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		sanitizedAttrs[i] = sanitizeAttr(a)
	}
	return &MaskingHandler{Handler: h.Handler.WithAttrs(sanitizedAttrs)}
}

func (h *MaskingHandler) WithGroup(name string) slog.Handler {
	return &MaskingHandler{Handler: h.Handler.WithGroup(name)}
}

func sanitizeAttr(a slog.Attr) slog.Attr {
	// If key indicates sensitive data, mask it completely
	lowerKey := strings.ToLower(a.Key)
	if strings.Contains(lowerKey, "token") ||
		strings.Contains(lowerKey, "secret") ||
		strings.Contains(lowerKey, "password") ||
		strings.Contains(lowerKey, "authorization") ||
		strings.Contains(lowerKey, "cookie") {
		return slog.String(a.Key, "***")
	}

	switch a.Value.Kind() {
	case slog.KindString:
		return slog.String(a.Key, MaskSensitiveString(a.Value.String()))
	case slog.KindGroup:
		attrs := a.Value.Group()
		sanitized := make([]slog.Attr, len(attrs))
		for i, subAttr := range attrs {
			sanitized[i] = sanitizeAttr(subAttr)
		}
		return slog.Group(a.Key, convertToAny(sanitized)...)
	default:
		return a
	}
}

func convertToAny(attrs []slog.Attr) []any {
	result := make([]any, len(attrs))
	for i, a := range attrs {
		result[i] = a
	}
	return result
}

// MaskSensitiveString sanitizes sensitive information in string payloads (URLs, headers, tokens).
func MaskSensitiveString(s string) string {
	if s == "" {
		return s
	}

	// Mask /config/{token} -> /config/***
	s = urlTokenRegex.ReplaceAllString(s, `${1}***`)

	// Mask Authorization header
	s = authHeaderRegex.ReplaceAllString(s, `${1}***`)

	// Mask Cookie header
	s = cookieRegex.ReplaceAllString(s, `${1}***`)

	// Mask password/secret/token/key query params
	s = passwordURIParam.ReplaceAllString(s, `${1}=***`)

	return s
}
