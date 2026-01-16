package cmd

import (
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/web"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	guiPort int
	guiOpen bool
)

var guiCmd = &cobra.Command{
	Use:     "gui",
	GroupID: GroupDiag,
	Short:   "Start the Gas Town web GUI",
	Long: `Start a web server that provides a full Gas Town control panel.

The GUI provides:
- Real-time status monitoring (daemon, rigs, convoys, polecats)
- Chat interface to communicate with Mayor
- Mail inbox viewer
- Command execution interface
- Auto-refresh every 30 seconds

Example:
  gt gui              # Start on default port 8080
  gt gui --port 3000  # Start on port 3000
  gt gui --open       # Start and open browser`,
	RunE: runGUI,
}

func init() {
	guiCmd.Flags().IntVar(&guiPort, "port", 8080, "HTTP port to listen on")
	guiCmd.Flags().BoolVar(&guiOpen, "open", false, "Open browser automatically")
	rootCmd.AddCommand(guiCmd)
}

func runGUI(cmd *cobra.Command, args []string) error {
	// Verify we're in a workspace
	if _, err := workspace.FindFromCwdOrError(); err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Create the live convoy fetcher
	fetcher, err := web.NewLiveConvoyFetcher()
	if err != nil {
		return fmt.Errorf("creating fetcher: %w", err)
	}

	// Create the GUI handler
	handler, err := web.NewGUIHandler(fetcher)
	if err != nil {
		return fmt.Errorf("creating GUI handler: %w", err)
	}

	// Build the URL
	url := fmt.Sprintf("http://localhost:%d", guiPort)

	// Open browser if requested
	if guiOpen {
		go openBrowser(url)
	}

	// Start the server with timeouts
	fmt.Printf("🏭 Gas Town GUI starting at %s\n", url)
	fmt.Printf("   Press Ctrl+C to stop\n")

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", guiPort),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return server.ListenAndServe()
}
