package web

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

func isSameOriginRequest(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}

	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}

	originHost, originPort := normalizeHostPort(parsed.Host)
	requestHost, requestPort := normalizeHostPort(r.Host)

	if originHost == "" || requestHost == "" {
		return false
	}
	if !strings.EqualFold(originHost, requestHost) {
		return false
	}
	if originPort == "" || requestPort == "" {
		return true
	}
	return originPort == requestPort
}

func normalizeHostPort(host string) (string, string) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", ""
	}
	parsedHost, port, err := net.SplitHostPort(host)
	if err == nil {
		return strings.Trim(parsedHost, "[]"), port
	}
	return strings.Trim(host, "[]"), ""
}
