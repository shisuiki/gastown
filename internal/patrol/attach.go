package patrol

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// IsHookEmpty checks if the specified agent's hook is empty.
// rootDir is the town root for deacon, or rig root for witness.
// agentAddr is the agent address (e.g., "deacon", "gastown/witness").
func IsHookEmpty(rootDir, agentAddr string) (bool, error) {
	cmd := exec.Command("gt", "hook", "show", agentAddr)
	cmd.Dir = rootDir
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to check hook status: %w", err)
	}
	// Output format: "agent: (empty)" or "agent: <bead-id> '<title>' [status]"
	outputStr := strings.TrimSpace(string(output))
	if strings.Contains(outputStr, ": (empty)") {
		return true, nil
	}
	return false, nil
}

// AttachPatrolMolecule attaches a patrol molecule to the specified agent.
// rootDir is the town root for deacon, or rig root for witness.
// agentAddr is the agent address (e.g., "deacon", "gastown/witness").
func AttachPatrolMolecule(rootDir, agentAddr string) error {
	// Determine bead directory
	beadsDir := filepath.Join(rootDir, ".beads")
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		return fmt.Errorf("beads directory not found: %s", beadsDir)
	}

	// Determine proto molecule name based on agent address
	var protoName string
	if strings.HasSuffix(agentAddr, "/witness") || agentAddr == "witness" {
		protoName = "mol-witness-patrol"
	} else if strings.HasSuffix(agentAddr, "/deacon") || agentAddr == "deacon" {
		protoName = "mol-deacon-patrol"
	} else if strings.HasSuffix(agentAddr, "/refinery") || agentAddr == "refinery" {
		protoName = "mol-refinery-patrol"
	} else {
		return fmt.Errorf("unknown agent role: %s", agentAddr)
	}

	// 1. Find proto ID for the patrol molecule
	catalogCmd := exec.Command("bd", "--no-daemon", "mol", "catalog")
	catalogCmd.Dir = beadsDir
	catalogOut, err := catalogCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list molecule catalog: %w", err)
	}

	var protoID string
	lines := strings.Split(string(catalogOut), "\n")
	for _, line := range lines {
		if strings.Contains(line, protoName) {
			parts := strings.Fields(line)
			if len(parts) > 0 {
				protoID = strings.TrimSuffix(parts[0], ":")
				break
			}
		}
	}
	if protoID == "" {
		return fmt.Errorf("proto %s not found in catalog", protoName)
	}

	// 2. Create patrol wisp
	// Use the agent address as actor (the assignee will be set later)
	spawnCmd := exec.Command("bd", "--no-daemon", "mol", "wisp", "create", protoID, "--actor", agentAddr)
	spawnCmd.Dir = beadsDir
	spawnOut, err := spawnCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to create patrol wisp: %w", err)
	}

	// Parse created molecule ID from output
	var patrolID string
	for _, line := range strings.Split(string(spawnOut), "\n") {
		if strings.Contains(line, "Root issue:") || strings.Contains(line, "Created") {
			parts := strings.Fields(line)
			for _, p := range parts {
				if strings.HasPrefix(p, "wisp-") || strings.HasPrefix(p, "gt-") {
					patrolID = p
					break
				}
			}
		}
	}
	if patrolID == "" {
		return fmt.Errorf("created wisp but could not parse ID from output")
	}

	// 3. Hook the wisp to the agent
	hookCmd := exec.Command("bd", "--no-daemon", "update", patrolID, "--status=hooked", "--assignee="+agentAddr)
	hookCmd.Dir = beadsDir
	if err := hookCmd.Run(); err != nil {
		return fmt.Errorf("failed to hook patrol wisp: %w", err)
	}

	return nil
}

// EnsurePatrolMoleculeAttached ensures that the agent has a patrol molecule attached.
// If the hook is empty, attaches one. Returns true if a molecule was attached.
func EnsurePatrolMoleculeAttached(rootDir, agentAddr string) (bool, error) {
	empty, err := IsHookEmpty(rootDir, agentAddr)
	if err != nil {
		return false, err
	}
	if empty {
		if err := AttachPatrolMolecule(rootDir, agentAddr); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}