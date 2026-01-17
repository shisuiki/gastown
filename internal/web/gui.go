package web

import (
	"embed"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
)

//go:embed templates/*.html
var templatesFS embed.FS

// GUIHandler handles the main Gas Town web GUI.
type GUIHandler struct {
	fetcher           ConvoyFetcher
	mux               *http.ServeMux
	allowPasswordAuth bool // For local dev only - logs warning if enabled
}

// authConfig controls authentication behavior.
// By default, only localhost connections are allowed.
// Set GT_WEB_AUTH_TOKEN env var to require token auth for all requests.
// Set GT_WEB_ALLOW_REMOTE=1 to allow non-localhost (use with reverse proxy auth).
var authConfig = struct {
	token       string
	allowRemote bool
}{
	token:       os.Getenv("GT_WEB_AUTH_TOKEN"),
	allowRemote: os.Getenv("GT_WEB_ALLOW_REMOTE") == "1",
}

// NewGUIHandler creates a new GUI handler with all routes.
func NewGUIHandler(fetcher ConvoyFetcher) (*GUIHandler, error) {
	h := &GUIHandler{
		fetcher: fetcher,
		mux:     http.NewServeMux(),
	}

	// Page routes
	h.mux.HandleFunc("/", h.handleDashboard)
	h.mux.HandleFunc("/mayor", h.handleMayor)
	h.mux.HandleFunc("/mail", h.handleMail)
	h.mux.HandleFunc("/terminals", h.handleTerminals)
	h.mux.HandleFunc("/activity", h.handleActivity)

	// Dashboard API routes
	h.mux.HandleFunc("/api/status", h.handleAPIStatus)
	h.mux.HandleFunc("/ws/status", h.handleStatusWS)

	// Mayor API routes
	h.mux.HandleFunc("/api/mayor/terminal", h.handleAPIMayorTerminal)
	h.mux.HandleFunc("/api/mayor/status", h.handleAPIMayorStatus)

	// Mail API routes
	h.mux.HandleFunc("/api/mail/send", h.handleAPISendMail)
	h.mux.HandleFunc("/api/mail/inbox", h.handleAPIMailInbox)
	h.mux.HandleFunc("/api/mail/all", h.handleAPIMailAll)
	h.mux.HandleFunc("/api/agents/list", h.handleAPIAgentsList)

	// Terminal API routes
	h.mux.HandleFunc("/api/terminal/stream", h.handleAPITerminalStream)

	// Activity API routes
	h.mux.HandleFunc("/api/activity", h.handleAPIActivity)

	// Shared API routes
	h.mux.HandleFunc("/api/command", h.handleAPICommand)
	h.mux.HandleFunc("/api/rigs", h.handleAPIRigs)
	h.mux.HandleFunc("/api/convoys", h.handleAPIConvoys)

	return h, nil
}

// ServeHTTP implements http.Handler with authentication middleware.
// Authentication requirements:
// - If GT_WEB_AUTH_TOKEN is set: requires Authorization: Bearer <token> header
// - If GT_WEB_ALLOW_REMOTE is not set: only allows localhost connections
// - Fails closed: rejects requests that don't meet auth requirements
func (h *GUIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check token auth if configured
	if authConfig.token != "" {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+authConfig.token {
			log.Printf("Auth failed: invalid or missing token from %s", r.RemoteAddr)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Check localhost unless remote explicitly allowed
	if !authConfig.allowRemote && !isLocalhost(r) {
		log.Printf("Auth failed: non-localhost request from %s (set GT_WEB_ALLOW_REMOTE=1 to allow)", r.RemoteAddr)
		http.Error(w, "Forbidden: localhost only", http.StatusForbidden)
		return
	}

	h.mux.ServeHTTP(w, r)
}

// isLocalhost checks if the request originates from localhost.
func isLocalhost(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	// Check common localhost representations
	if host == "127.0.0.1" || host == "::1" || host == "localhost" {
		return true
	}

	// Check if it's a loopback IP
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return true
	}

	// Also check X-Forwarded-For for reverse proxy setups
	// (only if GT_WEB_ALLOW_REMOTE is set, indicating proxy awareness)
	if authConfig.allowRemote {
		forwarded := r.Header.Get("X-Forwarded-For")
		if forwarded != "" {
			// Trust the first IP in the chain
			parts := strings.Split(forwarded, ",")
			if len(parts) > 0 {
				forwardedIP := net.ParseIP(strings.TrimSpace(parts[0]))
				if forwardedIP != nil && forwardedIP.IsLoopback() {
					return true
				}
			}
		}
	}

	return false
}
