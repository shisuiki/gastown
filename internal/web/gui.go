package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Cookie name for session authentication
const sessionCookieName = "gt_session"

// GUIHandler handles the main Gas Town web GUI.
type GUIHandler struct {
	fetcher           ConvoyFetcher
	mux               *http.ServeMux
	allowPasswordAuth bool // For local dev only - logs warning if enabled
	statusCache       *StatusCache
	cache             *Cache
}

// authConfig controls authentication behavior.
// By default, only localhost connections are allowed.
// Set GT_WEB_AUTH_TOKEN env var to require token auth for all requests.
// Set GT_WEB_ALLOW_REMOTE=1 to allow non-localhost (REQUIRES GT_WEB_AUTH_TOKEN).
var authConfig = struct {
	token       string
	allowRemote bool
}{
	token:       os.Getenv("GT_WEB_AUTH_TOKEN"),
	allowRemote: os.Getenv("GT_WEB_ALLOW_REMOTE") == "1",
}

// ErrInsecureRemoteConfig is returned when GT_WEB_ALLOW_REMOTE=1 is set without GT_WEB_AUTH_TOKEN.
var ErrInsecureRemoteConfig = errors.New("SECURITY: GT_WEB_ALLOW_REMOTE=1 requires GT_WEB_AUTH_TOKEN to be set")

// NewGUIHandler creates a new GUI handler with all routes.
// Returns ErrInsecureRemoteConfig if GT_WEB_ALLOW_REMOTE=1 is set without GT_WEB_AUTH_TOKEN.
func NewGUIHandler(fetcher ConvoyFetcher) (*GUIHandler, error) {
	// SECURITY: Reject insecure remote configuration
	if authConfig.allowRemote && authConfig.token == "" {
		return nil, ErrInsecureRemoteConfig
	}

	h := &GUIHandler{
		fetcher:     fetcher,
		mux:         http.NewServeMux(),
		statusCache: NewStatusCache(StatusCacheTTL),
		cache:       NewCache(),
	}

	// Static files (CSS, JS)
	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, err
	}
	h.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// Auth routes (these bypass the auth middleware)
	h.mux.HandleFunc("/login", h.handleLogin)
	h.mux.HandleFunc("/logout", h.handleLogout)

	// Page routes
	h.mux.HandleFunc("/", h.handleRootRedirect)
	h.mux.HandleFunc("/dashboard", h.handleDashboard)
	h.mux.HandleFunc("/mayor", h.handleMayor)
	h.mux.HandleFunc("/mail", h.handleMail)
	h.mux.HandleFunc("/terminals", h.handleTerminals)
	h.mux.HandleFunc("/workflow", h.handleWorkflow)
	h.mux.HandleFunc("/activity", h.handleWorkflow) // Legacy redirect
	h.mux.HandleFunc("/git", h.handleGit)
	h.mux.HandleFunc("/config", h.handleConfig)
	h.mux.HandleFunc("/prompts", h.handlePrompts)

	// Detail page routes (prefix matching)
	h.mux.HandleFunc("/convoy/", h.handleConvoyDetail)
	h.mux.HandleFunc("/bead/", h.handleBeadDetail)

	// Dashboard API routes
	h.mux.HandleFunc("/api/status", h.handleAPIStatus)
	h.mux.HandleFunc("/ws/status", h.handleStatusWS)
	h.mux.HandleFunc("/api/issues", h.handleAPIIssues)
	h.mux.HandleFunc("/api/role-beads", h.handleAPIRoleBeads)

	// Mayor API routes
	h.mux.HandleFunc("/api/mayor/terminal", h.handleAPIMayorTerminal)
	h.mux.HandleFunc("/api/mayor/status", h.handleAPIMayorStatus)

	// Mail API routes
	h.mux.HandleFunc("/api/mail/send", h.handleAPISendMail)
	h.mux.HandleFunc("/api/mail/inbox", h.handleAPIMailInbox)
	h.mux.HandleFunc("/api/mail/all", h.handleAPIMailAll)
	h.mux.HandleFunc("/api/agents/list", h.handleAPIAgentsList)
	h.mux.HandleFunc("/api/mail/mark-read", h.handleAPIMailMarkRead)
	h.mux.HandleFunc("/api/mail/mark-unread", h.handleAPIMailMarkUnread)

	// Terminal API routes
	h.mux.HandleFunc("/api/terminal/stream", h.handleAPITerminalStream)
	h.mux.HandleFunc("/api/terminal/send", h.handleAPITerminalSend)

	// Workflow API routes
	h.mux.HandleFunc("/api/activity", h.handleAPIActivity)        // Legacy: git commits
	h.mux.HandleFunc("/api/workflow/hook", h.handleAPIWorkflowHook)
	h.mux.HandleFunc("/api/workflow/ready", h.handleAPIWorkflowReady)

	// Detail API routes (prefix matching)
	h.mux.HandleFunc("/api/convoy/", h.handleAPIConvoyDetail)
	h.mux.HandleFunc("/api/bead/", h.handleAPIBeadDetail)

	// Quick action API routes
	h.mux.HandleFunc("/api/action", h.handleAPIActions)
	h.mux.HandleFunc("/api/convoy/create", h.handleAPICreateConvoy)
	h.mux.HandleFunc("/api/bead/create", h.handleAPICreateBead)

	// System and Git API routes
	h.mux.HandleFunc("/api/version", h.handleAPIVersion)
	h.mux.HandleFunc("/api/system", h.handleAPISystem)
	h.mux.HandleFunc("/api/claude/usage", h.handleAPIClaudeUsage)
	h.mux.HandleFunc("/api/git/rigs", h.handleAPIGitRigs)
	h.mux.HandleFunc("/api/git/commits", h.handleAPIGitCommits)
	h.mux.HandleFunc("/api/git/branches", h.handleAPIGitBranches)
	h.mux.HandleFunc("/api/git/graph", h.handleAPIGitGraph)

	// Config API routes
	h.mux.HandleFunc("/api/config", h.handleAPIConfig)
	h.mux.HandleFunc("/api/prompts/", h.handleAPIPrompts)

	// Shared API routes
	h.mux.HandleFunc("/api/command", h.handleAPICommand)
	h.mux.HandleFunc("/api/rigs", h.handleAPIRigs)
	h.mux.HandleFunc("/api/convoys", h.handleAPIConvoys)

	return h, nil
}

// ServeHTTP implements http.Handler with authentication middleware.
// Authentication requirements:
// - If GT_WEB_AUTH_TOKEN is set: requires valid session cookie OR Authorization: Bearer <token> header
// - If GT_WEB_ALLOW_REMOTE is not set: only allows localhost connections
// - Fails closed: rejects requests that don't meet auth requirements
func (h *GUIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Login/logout pages and static files bypass auth
	if r.URL.Path == "/login" || r.URL.Path == "/logout" || strings.HasPrefix(r.URL.Path, "/static/") {
		h.mux.ServeHTTP(w, r)
		return
	}

	// Check token auth if configured
	if authConfig.token != "" {
		if !h.isAuthenticated(r) {
			// For browser page requests, redirect to login page
			// Only redirect for explicit HTML requests, not API/WebSocket
			if isPageRequest(r) {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
			// For API/WebSocket requests, return 401
			log.Printf("Auth failed: invalid or missing token from %s for %s", r.RemoteAddr, r.URL.Path)
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

// isAuthenticated checks if the request has valid authentication.
// Supports both cookie-based and header-based authentication.
func (h *GUIHandler) isAuthenticated(r *http.Request) bool {
	// Check Authorization header
	auth := r.Header.Get("Authorization")
	if auth == "Bearer "+authConfig.token {
		return true
	}

	// Check session cookie
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value == generateSessionToken(authConfig.token) {
		return true
	}

	return false
}

// isHTMLRequest checks if the request expects an HTML response.
func isHTMLRequest(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "text/html") || accept == "" || accept == "*/*"
}

// isPageRequest checks if this is a browser page navigation request.
// More strict than isHTMLRequest - only redirects for explicit page requests.
func isPageRequest(r *http.Request) bool {
	// WebSocket upgrade requests should never redirect
	if r.Header.Get("Upgrade") == "websocket" {
		return false
	}

	// API and data requests should not redirect
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws/") {
		return false
	}

	// Check Accept header for HTML
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "text/html")
}

// generateSessionToken creates a session token from the auth token.
// This is a simple hash to avoid exposing the raw token in cookies.
func generateSessionToken(token string) string {
	hash := sha256.Sum256([]byte("gt-session:" + token))
	return hex.EncodeToString(hash[:])
}

// handleLogin serves the login page and handles login form submission.
func (h *GUIHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		token := r.FormValue("token")
		if token == authConfig.token {
			// Set session cookie
			http.SetCookie(w, &http.Cookie{
				Name:     sessionCookieName,
				Value:    generateSessionToken(token),
				Path:     "/",
				HttpOnly: true,
				Secure:   r.TLS != nil,
				SameSite: http.SameSiteLaxMode,
				MaxAge:   86400 * 30, // 30 days
			})
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		// Show login page with error
		h.serveLoginPage(w, "Invalid token")
		return
	}

	// GET request - show login page
	h.serveLoginPage(w, "")
}

// handleLogout clears the session cookie.
func (h *GUIHandler) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1, // Delete cookie
	})
	http.Redirect(w, r, "/login", http.StatusFound)
}

// serveLoginPage serves the login HTML page.
func (h *GUIHandler) serveLoginPage(w http.ResponseWriter, errorMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	errorHTML := ""
	if errorMsg != "" {
		errorHTML = `<div class="error">` + errorMsg + `</div>`
	}

	html := `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Gas Town - Login</title>
    <style>
        * { box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            background: linear-gradient(135deg, #1a1a2e 0%, #16213e 100%);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            margin: 0;
            padding: 20px;
        }
        .login-container {
            background: rgba(255, 255, 255, 0.1);
            backdrop-filter: blur(10px);
            border-radius: 16px;
            padding: 40px;
            width: 100%;
            max-width: 400px;
            box-shadow: 0 8px 32px rgba(0, 0, 0, 0.3);
        }
        h1 {
            color: #fff;
            text-align: center;
            margin: 0 0 8px 0;
            font-size: 28px;
        }
        .subtitle {
            color: rgba(255, 255, 255, 0.6);
            text-align: center;
            margin-bottom: 32px;
            font-size: 14px;
        }
        .form-group {
            margin-bottom: 20px;
        }
        label {
            display: block;
            color: rgba(255, 255, 255, 0.8);
            margin-bottom: 8px;
            font-size: 14px;
        }
        input[type="password"] {
            width: 100%;
            padding: 14px 16px;
            border: 1px solid rgba(255, 255, 255, 0.2);
            border-radius: 8px;
            background: rgba(255, 255, 255, 0.1);
            color: #fff;
            font-size: 16px;
            transition: border-color 0.2s, background 0.2s;
        }
        input[type="password"]:focus {
            outline: none;
            border-color: #4f8cff;
            background: rgba(255, 255, 255, 0.15);
        }
        input[type="password"]::placeholder {
            color: rgba(255, 255, 255, 0.4);
        }
        button {
            width: 100%;
            padding: 14px;
            background: linear-gradient(135deg, #4f8cff 0%, #3b6fd4 100%);
            border: none;
            border-radius: 8px;
            color: #fff;
            font-size: 16px;
            font-weight: 600;
            cursor: pointer;
            transition: transform 0.2s, box-shadow 0.2s;
        }
        button:hover {
            transform: translateY(-2px);
            box-shadow: 0 4px 20px rgba(79, 140, 255, 0.4);
        }
        button:active {
            transform: translateY(0);
        }
        .error {
            background: rgba(255, 107, 107, 0.2);
            border: 1px solid rgba(255, 107, 107, 0.4);
            color: #ff6b6b;
            padding: 12px 16px;
            border-radius: 8px;
            margin-bottom: 20px;
            text-align: center;
            font-size: 14px;
        }
        .hint {
            color: rgba(255, 255, 255, 0.5);
            font-size: 12px;
            text-align: center;
            margin-top: 20px;
        }
    </style>
</head>
<body>
    <div class="login-container">
        <h1>Gas Town</h1>
        <p class="subtitle">Multi-Agent Workspace Manager</p>
        ` + errorHTML + `
        <form method="POST" action="/login">
            <div class="form-group">
                <label for="token">Access Token</label>
                <input type="password" id="token" name="token" placeholder="Enter GT_WEB_AUTH_TOKEN" required autofocus>
            </div>
            <button type="submit">Sign In</button>
        </form>
        <p class="hint">Token is set via GT_WEB_AUTH_TOKEN environment variable</p>
    </div>
</body>
</html>`
	w.Write([]byte(html))
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
