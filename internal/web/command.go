package web

import (
	"context"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

const (
	defaultCommandTimeout = 15 * time.Second
	longCommandTimeout    = 45 * time.Second
)

func command(name string, args ...string) (*exec.Cmd, context.CancelFunc) {
	return commandWithTimeout(defaultCommandTimeout, name, args...)
}

func longCommand(name string, args ...string) (*exec.Cmd, context.CancelFunc) {
	return commandWithTimeout(longCommandTimeout, name, args...)
}

func ghCommand(args ...string) (*exec.Cmd, context.CancelFunc) {
	cmd, cancel := longCommand("gh", args...)
	cmd.Env = applyEnvOverrides(os.Environ(), proxyEnvOverrides())
	return cmd, cancel
}

func commandWithTimeout(timeout time.Duration, name string, args ...string) (*exec.Cmd, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	return exec.CommandContext(ctx, name, args...), cancel
}

func proxyEnvOverrides() map[string]string {
	overrides := map[string]string{}
	setProxyOverride(overrides, "HTTP_PROXY", "GT_WEB_HTTP_PROXY")
	setProxyOverride(overrides, "HTTPS_PROXY", "GT_WEB_HTTPS_PROXY")
	setProxyOverride(overrides, "ALL_PROXY", "GT_WEB_ALL_PROXY")
	setProxyOverride(overrides, "NO_PROXY", "GT_WEB_NO_PROXY")
	return overrides
}

func setProxyOverride(overrides map[string]string, targetKey, sourceKey string) {
	if val := strings.TrimSpace(os.Getenv(sourceKey)); val != "" {
		overrides[targetKey] = val
		overrides[strings.ToLower(targetKey)] = val
	}
}

func applyEnvOverrides(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return base
	}

	envMap := make(map[string]string, len(base)+len(overrides))
	for _, pair := range base {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	for key, val := range overrides {
		envMap[key] = val
	}

	merged := make([]string, 0, len(envMap))
	for key, val := range envMap {
		merged = append(merged, key+"="+val)
	}
	sort.Strings(merged)
	return merged
}
