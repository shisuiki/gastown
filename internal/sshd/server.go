// Package sshd provides SSH server functionality for Gas Town agents.
// This allows crew members and other agents to have their own SSH endpoints
// for remote access and collaboration.
package sshd

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

// Server represents an SSH server for a Gas Town agent.
type Server struct {
	config     *ssh.ServerConfig
	listener   net.Listener
	port       int
	agentName  string
	agentRole  string
	workDir    string
	hostKey    ssh.Signer
	authorizedKeys map[string]bool
	mu         sync.RWMutex
	running    bool

	// Security options
	allowPasswordAuth  bool
	allowShellCommands bool
}

// Config holds SSH server configuration.
type Config struct {
	Port           int
	AgentName      string
	AgentRole      string // crew, polecat, etc.
	WorkDir        string
	HostKeyPath    string
	AuthorizedKeys []string

	// Security options - both default to false for security
	AllowPasswordAuth bool // If true, accept any password (DEV ONLY - insecure)
	AllowShellCommands bool // If true, enable run/sh/bash commands (DEV ONLY - insecure)
}

// New creates a new SSH server for an agent.
func New(cfg Config) (*Server, error) {
	// Load or generate host key
	hostKey, err := loadOrGenerateHostKey(cfg.HostKeyPath)
	if err != nil {
		return nil, fmt.Errorf("loading host key: %w", err)
	}

	s := &Server{
		port:               cfg.Port,
		agentName:          cfg.AgentName,
		agentRole:          cfg.AgentRole,
		workDir:            cfg.WorkDir,
		hostKey:            hostKey,
		authorizedKeys:     make(map[string]bool),
		allowPasswordAuth:  cfg.AllowPasswordAuth,
		allowShellCommands: cfg.AllowShellCommands,
	}

	// Log warnings when insecure modes are enabled
	if cfg.AllowPasswordAuth {
		log.Printf("WARNING: SSH password authentication enabled for %s - DEV MODE ONLY, accepts any password", cfg.AgentName)
	}
	if cfg.AllowShellCommands {
		log.Printf("WARNING: SSH shell commands (run/sh/bash) enabled for %s - DEV MODE ONLY, allows arbitrary command execution", cfg.AgentName)
	}

	// Parse authorized keys
	for _, key := range cfg.AuthorizedKeys {
		pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(key))
		if err != nil {
			log.Printf("Warning: invalid authorized key: %v", err)
			continue
		}
		s.authorizedKeys[string(pubKey.Marshal())] = true
	}

	// Setup SSH config
	s.config = &ssh.ServerConfig{
		PublicKeyCallback: s.publicKeyCallback,
		PasswordCallback:  s.passwordCallback,
	}
	s.config.AddHostKey(hostKey)

	return s, nil
}

// Start starts the SSH server.
func (s *Server) Start() error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", s.port, err)
	}

	s.mu.Lock()
	s.listener = listener
	s.running = true
	s.mu.Unlock()

	log.Printf("SSH server for %s started on port %d", s.agentName, s.port)

	go s.acceptLoop()
	return nil
}

// Stop stops the SSH server.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

// Port returns the server's port.
func (s *Server) Port() int {
	return s.port
}

// IsRunning returns whether the server is running.
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.mu.RLock()
			running := s.running
			s.mu.RUnlock()
			if !running {
				return
			}
			log.Printf("Accept error: %v", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	sshConn, chans, reqs, err := ssh.NewServerConn(conn, s.config)
	if err != nil {
		log.Printf("SSH handshake failed: %v", err)
		return
	}
	defer sshConn.Close()

	log.Printf("SSH connection from %s (%s)", sshConn.RemoteAddr(), sshConn.User())

	// Discard global requests
	go ssh.DiscardRequests(reqs)

	// Handle channels
	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			log.Printf("Could not accept channel: %v", err)
			continue
		}

		go s.handleSession(channel, requests)
	}
}

func (s *Server) handleSession(channel ssh.Channel, requests <-chan *ssh.Request) {
	defer channel.Close()

	for req := range requests {
		switch req.Type {
		case "shell":
			req.Reply(true, nil)
			s.handleShell(channel)
			return

		case "exec":
			req.Reply(true, nil)
			var payload struct{ Command string }
			ssh.Unmarshal(req.Payload, &payload)
			s.handleExec(channel, payload.Command)
			return

		case "pty-req":
			req.Reply(true, nil)

		default:
			req.Reply(false, nil)
		}
	}
}

func (s *Server) handleShell(channel ssh.Channel) {
	// Interactive shell - show prompt and handle commands
	prompt := fmt.Sprintf("[%s@%s]$ ", s.agentRole, s.agentName)

	channel.Write([]byte(fmt.Sprintf("Welcome to Gas Town SSH - %s (%s)\n", s.agentName, s.agentRole)))
	channel.Write([]byte("Type 'help' for available commands, 'exit' to disconnect\n\n"))

	buf := make([]byte, 1024)
	var cmdBuf strings.Builder

	channel.Write([]byte(prompt))

	for {
		n, err := channel.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Printf("Read error: %v", err)
			}
			return
		}

		for i := 0; i < n; i++ {
			b := buf[i]

			switch b {
			case '\r', '\n':
				channel.Write([]byte("\r\n"))
				cmd := strings.TrimSpace(cmdBuf.String())
				cmdBuf.Reset()

				if cmd == "exit" || cmd == "quit" {
					channel.Write([]byte("Goodbye!\n"))
					return
				}

				if cmd != "" {
					output := s.executeCommand(cmd)
					channel.Write([]byte(output))
					if !strings.HasSuffix(output, "\n") {
						channel.Write([]byte("\n"))
					}
				}
				channel.Write([]byte(prompt))

			case 127, '\b': // Backspace
				if cmdBuf.Len() > 0 {
					str := cmdBuf.String()
					cmdBuf.Reset()
					cmdBuf.WriteString(str[:len(str)-1])
					channel.Write([]byte("\b \b"))
				}

			case 3: // Ctrl+C
				cmdBuf.Reset()
				channel.Write([]byte("^C\n" + prompt))

			case 4: // Ctrl+D
				channel.Write([]byte("\nGoodbye!\n"))
				return

			default:
				if b >= 32 && b < 127 {
					cmdBuf.WriteByte(b)
					channel.Write([]byte{b})
				}
			}
		}
	}
}

func (s *Server) handleExec(channel ssh.Channel, command string) {
	output := s.executeCommand(command)
	channel.Write([]byte(output))

	// Send exit status
	exitStatus := []byte{0, 0, 0, 0}
	channel.SendRequest("exit-status", false, exitStatus)
}

func (s *Server) executeCommand(command string) string {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return ""
	}

	cmd := parts[0]
	args := parts[1:]

	// Built-in commands
	switch cmd {
	case "help":
		return s.helpText()

	case "status":
		return s.runGT("daemon", "status")

	case "mail":
		if len(args) == 0 {
			return s.runGT("mail", "inbox")
		}
		return s.runGT(append([]string{"mail"}, args...)...)

	case "hook":
		return s.runGT("hook")

	case "ready":
		return s.runGT("ready")

	case "convoy":
		return s.runGT(append([]string{"convoy"}, args...)...)

	case "rig":
		return s.runGT(append([]string{"rig"}, args...)...)

	case "trail":
		return s.runGT("trail")

	case "gt":
		// Direct gt command passthrough (safe commands only)
		if len(args) > 0 {
			return s.runGT(args...)
		}
		return "Usage: gt <command> [args...]"

	case "pwd":
		return s.workDir + "\n"

	case "whoami":
		return fmt.Sprintf("%s (%s)\n", s.agentName, s.agentRole)

	case "ls":
		return s.runCmd("ls", args...)

	case "cat":
		return s.runCmd("cat", args...)

	case "git":
		return s.runCmd("git", args...)

	case "run", "sh", "bash":
		// Shell commands are disabled by default for security
		if !s.allowShellCommands {
			return "Error: Shell commands (run/sh/bash) are disabled for security.\nContact your administrator to enable AllowShellCommands if needed for development.\n"
		}
		// Execute arbitrary shell command via bash -c
		// This allows full shell syntax including &&, |, ;, etc.
		if len(args) == 0 {
			return "Usage: run <command>\nExample: run ls -la && pwd\n"
		}
		log.Printf("WARNING: Executing shell command: %s - DEV MODE", strings.Join(args, " "))
		// Join args back into a single command string
		shellCmd := strings.Join(args, " ")
		return s.runShell(shellCmd)

	case "cd":
		// Change working directory for subsequent commands
		if len(args) == 0 {
			return "Usage: cd <directory>\n"
		}
		newDir := args[0]
		if !filepath.IsAbs(newDir) {
			newDir = filepath.Join(s.workDir, newDir)
		}
		// Verify directory exists
		if info, err := os.Stat(newDir); err != nil || !info.IsDir() {
			return fmt.Sprintf("cd: %s: No such directory\n", args[0])
		}
		s.workDir = newDir
		return ""

	default:
		return fmt.Sprintf("Unknown command: %s\nType 'help' for available commands\n", cmd)
	}
}

func (s *Server) runGT(args ...string) string {
	cmd := exec.Command("gt", args...)
	cmd.Dir = s.workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("%s\nError: %v\n", output, err)
	}
	return string(output)
}

func (s *Server) runCmd(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	cmd.Dir = s.workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("%s\nError: %v\n", output, err)
	}
	return string(output)
}

func (s *Server) runShell(command string) string {
	cmd := exec.Command("bash", "-c", command)
	cmd.Dir = s.workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("%s\nError: %v\n", output, err)
	}
	return string(output)
}

func (s *Server) helpText() string {
	shellStatus := "(disabled - requires AllowShellCommands)"
	if s.allowShellCommands {
		shellStatus = "(enabled - DEV MODE)"
	}

	return fmt.Sprintf(`Gas Town SSH Commands:
  status     - Show daemon status
  mail       - Check inbox (mail inbox, mail send, etc.)
  hook       - Show current hook status
  ready      - Show ready work
  convoy     - Convoy management
  rig        - Rig management
  trail      - Show recent activity
  gt <cmd>   - Run any gt command

  pwd        - Show working directory
  whoami     - Show agent identity
  cd <dir>   - Change working directory
  ls [path]  - List files
  cat <file> - Show file contents
  git <cmd>  - Run git commands

  run <cmd>  - Execute shell command %s
               Example: run ls -la && pwd
               Aliases: sh, bash

  help       - Show this help
  exit       - Disconnect
`, shellStatus)
}

func (s *Server) publicKeyCallback(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.authorizedKeys[string(key.Marshal())] {
		return &ssh.Permissions{
			Extensions: map[string]string{
				"user": conn.User(),
			},
		}, nil
	}
	return nil, fmt.Errorf("unknown public key for %q", conn.User())
}

func (s *Server) passwordCallback(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
	// Password auth is disabled by default for security
	// Only enabled when AllowPasswordAuth is explicitly set (dev mode only)
	if !s.allowPasswordAuth {
		return nil, fmt.Errorf("password authentication disabled")
	}

	log.Printf("WARNING: Accepting password auth from %s (user: %s) - DEV MODE", conn.RemoteAddr(), conn.User())
	return &ssh.Permissions{
		Extensions: map[string]string{
			"user": conn.User(),
		},
	}, nil
}

func loadOrGenerateHostKey(path string) (ssh.Signer, error) {
	if path == "" {
		path = filepath.Join(os.TempDir(), "gastown_host_key")
	}

	// Try to load existing key
	keyBytes, err := os.ReadFile(path)
	if err == nil {
		signer, err := ssh.ParsePrivateKey(keyBytes)
		if err == nil {
			return signer, nil
		}
	}

	// Generate new key
	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-f", path, "-N", "")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("generating host key: %w", err)
	}

	keyBytes, err = os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading generated key: %w", err)
	}

	return ssh.ParsePrivateKey(keyBytes)
}
