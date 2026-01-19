package web

import (
	"context"
	"os/exec"
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

func commandWithTimeout(timeout time.Duration, name string, args ...string) (*exec.Cmd, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	return exec.CommandContext(ctx, name, args...), cancel
}
