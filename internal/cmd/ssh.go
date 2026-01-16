package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/sshd"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	sshPort    int
	sshKeyFile string
)

var sshCmd = &cobra.Command{
	Use:     "ssh",
	GroupID: GroupAgents,
	Short:   "SSH server commands for agent endpoints",
	Long: `Manage SSH endpoints for Gas Town agents.

Each crew member or agent can have their own SSH endpoint for remote access.
This allows distributed team workflows and remote collaboration.`,
}

var sshStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start SSH server for this agent",
	Long: `Start an SSH server that provides remote access to this agent's workspace.

Users can connect via:
  ssh -p <port> <username>@<host>

The SSH shell provides access to Gas Town commands (mail, convoy, rig, etc.)
and basic file operations.

Example:
  gt ssh start              # Start on default port 2222
  gt ssh start --port 2223  # Start on custom port`,
	RunE: runSSHStart,
}

var sshStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop SSH server",
	RunE:  runSSHStop,
}

var sshStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show SSH server status",
	RunE:  runSSHStatus,
}

func init() {
	sshStartCmd.Flags().IntVar(&sshPort, "port", 2222, "SSH port to listen on")
	sshStartCmd.Flags().StringVar(&sshKeyFile, "key", "", "Path to host key file")

	sshCmd.AddCommand(sshStartCmd)
	sshCmd.AddCommand(sshStopCmd)
	sshCmd.AddCommand(sshStatusCmd)
	rootCmd.AddCommand(sshCmd)
}

func runSSHStart(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Determine agent info from environment or role
	agentName := os.Getenv("GT_AGENT_NAME")
	agentRole := os.Getenv("GT_ROLE")
	if agentName == "" {
		agentName = "gastown"
	}
	if agentRole == "" {
		agentRole = "agent"
	}

	// Default host key path
	if sshKeyFile == "" {
		sshKeyFile = filepath.Join(townRoot, ".ssh", "host_key")
		os.MkdirAll(filepath.Dir(sshKeyFile), 0700)
	}

	// Load authorized keys from ~/.ssh/authorized_keys
	var authKeys []string
	authKeysPath := filepath.Join(os.Getenv("HOME"), ".ssh", "authorized_keys")
	if data, err := os.ReadFile(authKeysPath); err == nil {
		authKeys = append(authKeys, string(data))
	}

	// Create and start server
	server, err := sshd.New(sshd.Config{
		Port:           sshPort,
		AgentName:      agentName,
		AgentRole:      agentRole,
		WorkDir:        townRoot,
		HostKeyPath:    sshKeyFile,
		AuthorizedKeys: authKeys,
	})
	if err != nil {
		return fmt.Errorf("creating SSH server: %w", err)
	}

	if err := server.Start(); err != nil {
		return fmt.Errorf("starting SSH server: %w", err)
	}

	fmt.Printf("🔐 SSH server started for %s (%s)\n", agentName, agentRole)
	fmt.Printf("   Port: %d\n", sshPort)
	fmt.Printf("   Connect: ssh -p %d localhost\n", sshPort)
	fmt.Printf("\n   Press Ctrl+C to stop\n")

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down SSH server...")
	return server.Stop()
}

func runSSHStop(cmd *cobra.Command, args []string) error {
	// TODO: Implement stop via PID file or signal
	fmt.Println("SSH server stop not yet implemented")
	fmt.Println("Use Ctrl+C in the terminal running 'gt ssh start'")
	return nil
}

func runSSHStatus(cmd *cobra.Command, args []string) error {
	// TODO: Check if server is running via PID file
	fmt.Println("SSH server status not yet implemented")
	return nil
}
