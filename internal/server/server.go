package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"mihomo-sub-publisher/internal/config"
	"mihomo-sub-publisher/internal/generator"
	"mihomo-sub-publisher/internal/logging"
	"mihomo-sub-publisher/internal/token"
)

// StatusResponse is returned by /status.
type StatusResponse struct {
	Status            string    `json:"status"`
	SnapshotVersion   uint64    `json:"snapshot_version,omitempty"`
	SourceUpdatedAt   time.Time `json:"source_updated_at,omitempty"`
	GeneratedAt       time.Time `json:"generated_at,omitempty"`
	LastUpdateSuccess bool      `json:"last_update_success"`
}

// Server handles incoming HTTP requests.
type Server struct {
	httpServer  *http.Server
	cfg         *config.Config
	tokenStore  *token.Store
	snapshotMgr *generator.SnapshotManager
	listener    net.Listener
}

// NewServer creates a new HTTP Server.
func NewServer(
	cfg *config.Config,
	tokenStore *token.Store,
	snapshotMgr *generator.SnapshotManager,
) *Server {
	s := &Server{
		cfg:         cfg,
		tokenStore:  tokenStore,
		snapshotMgr: snapshotMgr,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/status", s.handleStatus)

	// Register config route matching template prefix
	// Default template is "/config/{token}"
	prefix, _ := parseConfigPathPrefix(cfg.Server.ConfigPath)
	mux.HandleFunc(prefix, s.handleConfig)

	s.httpServer = &http.Server{
		Addr:         cfg.Server.Listen,
		Handler:      s.loggingMiddleware(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

func parseConfigPathPrefix(pattern string) (prefix string, isTemplate bool) {
	if idx := strings.Index(pattern, "{token}"); idx != -1 {
		return pattern[:idx], true
	}
	return pattern, false
}

// Start runs the HTTP server.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.cfg.Server.Listen)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.cfg.Server.Listen, err)
	}
	s.listener = ln

	logger := logging.Logger()
	logger.Info("http server listening", "addr", s.cfg.Server.Listen)

	go func() {
		if err := s.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server error", "error", err)
		}
	}()

	return nil
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// Addr returns the actual listener address (useful for dynamic port in tests).
func (s *Server) Addr() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.cfg.Server.Listen
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sanitizedURI := logging.MaskSensitiveString(r.RequestURI)

		rw := &responseWriterTracker{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r)

		duration := time.Since(start)
		logger := logging.Logger()
		logger.Info("http.request",
			"method", r.Method,
			"uri", sanitizedURI,
			"status", rw.statusCode,
			"duration_ms", duration.Milliseconds(),
			"remote_addr", r.RemoteAddr,
		)
	})
}

type responseWriterTracker struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriterTracker) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	snap := s.snapshotMgr.Get()
	var res StatusResponse
	if snap != nil {
		res = StatusResponse{
			Status:            "ready",
			SnapshotVersion:   snap.Version,
			SourceUpdatedAt:   snap.SourceUpdatedAt,
			GeneratedAt:       snap.GeneratedAt,
			LastUpdateSuccess: true,
		}
	} else {
		res = StatusResponse{
			Status:            "NO_SNAPSHOT",
			LastUpdateSuccess: false,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	// Validate headers: only clash-like User-Agents and expected headers are accepted.
	// Unexpected headers return 404 Not Found.
	if !s.isExpectedRequest(r) {
		s.writeJSONError(w, http.StatusNotFound, "not_found")
		return
	}

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		s.writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}

	prefix, _ := parseConfigPathPrefix(s.cfg.Server.ConfigPath)
	path := r.URL.Path
	if !strings.HasPrefix(path, prefix) {
		s.writeJSONError(w, http.StatusNotFound, "not_found")
		return
	}

	tokenStr := strings.TrimPrefix(path, prefix)
	tokenStr = strings.Trim(tokenStr, "/")
	if tokenStr == "" {
		s.writeJSONError(w, http.StatusNotFound, "not_found")
		return
	}

	// 1. Authorize Token
	tokenItem, err := s.tokenStore.Authorize(tokenStr)
	if err != nil {
		logger := logging.Logger()
		logger.Warn("token.download.denied", "reason", err.Error())

		if errors.Is(err, token.ErrTokenNotFound) {
			s.writeJSONError(w, http.StatusNotFound, "not_found")
		} else if errors.Is(err, token.ErrTokenExpired) || errors.Is(err, token.ErrTokenExhausted) {
			s.writeJSONError(w, http.StatusForbidden, "forbidden")
		} else {
			s.writeJSONError(w, http.StatusForbidden, "forbidden")
		}
		return
	}

	// 2. Check Snapshot
	snap := s.snapshotMgr.Get()
	if snap == nil {
		s.writeJSONError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}

	// 3. Write Response Headers
	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("X-Source-Updated-At", snap.SourceUpdatedAt.UTC().Format(time.RFC3339))
	w.Header().Set("X-Generated-At", snap.GeneratedAt.UTC().Format(time.RFC3339))
	w.Header().Set("X-Config-SHA256", snap.SHA256)

	// Forward saved upstream subscription headers (subscription-userinfo, profile-update-interval, content-disposition, etc.)
	for k, v := range snap.Headers {
		w.Header().Set(k, v)
	}

	w.WriteHeader(http.StatusOK)

	// 4. Write Body
	if r.Method == http.MethodGet {
		if _, err := w.Write(snap.Content); err == nil {
			// Deduct quota after successful response write
			s.tokenStore.Deduct(tokenItem)
			logger := logging.Logger()
			logger.Info("token.download.success", "token_name", tokenItem.Definition.Name, "remaining", tokenItem.Remaining.Load())
		}
	}
}

func (s *Server) isExpectedRequest(r *http.Request) bool {
	// 1. User-Agent check (must be a Clash-compatible client)
	ua := strings.TrimSpace(r.Header.Get("User-Agent"))
	if ua == "" {
		return false
	}
	lowerUA := strings.ToLower(ua)

	uaMatched := false
	if len(s.cfg.Server.AllowedUserAgents) > 0 {
		for _, pattern := range s.cfg.Server.AllowedUserAgents {
			lowerPat := strings.ToLower(strings.TrimSpace(pattern))
			if lowerPat == "*" {
				uaMatched = true
				break
			}
			if strings.HasPrefix(lowerPat, "*") && strings.HasSuffix(lowerPat, "*") && len(lowerPat) > 2 {
				sub := lowerPat[1 : len(lowerPat)-1]
				if strings.Contains(lowerUA, sub) {
					uaMatched = true
					break
				}
			} else if matched, _ := filepath.Match(lowerPat, lowerUA); matched {
				uaMatched = true
				break
			} else if strings.Contains(lowerUA, lowerPat) {
				uaMatched = true
				break
			}
		}
	} else {
		uaMatched = strings.Contains(lowerUA, "clash") ||
			strings.Contains(lowerUA, "mihomo") ||
			strings.Contains(lowerUA, "stash")
	}

	if !uaMatched {
		return false
	}

	// 2. Required headers check (if configured)
	for reqKey, reqVal := range s.cfg.Server.RequiredHeaders {
		actualVal := r.Header.Get(reqKey)
		if actualVal == "" {
			return false
		}
		if reqVal != "" && reqVal != "*" {
			if !strings.EqualFold(actualVal, reqVal) {
				return false
			}
		}
	}

	return true
}

func (s *Server) writeJSONError(w http.ResponseWriter, code int, errCode string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(fmt.Sprintf(`{"error":%q}`, errCode)))
}
