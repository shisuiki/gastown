package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
)

const csrfCookieName = "gt_csrf"

func isStateChangingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func ensureCSRFCookie(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(csrfCookieName); err == nil && cookie.Value != "" {
		return
	}

	token, err := newCSRFToken()
	if err != nil {
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400 * 30, // 30 days
	})
}

func validateCSRF(r *http.Request) bool {
	cookie, err := r.Cookie(csrfCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}

	header := r.Header.Get("X-CSRF-Token")
	if header == "" {
		return false
	}

	if len(header) != len(cookie.Value) {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(header), []byte(cookie.Value)) == 1
}

func newCSRFToken() (string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	return hex.EncodeToString(secret), nil
}
