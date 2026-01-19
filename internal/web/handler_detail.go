package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// DetailPageData is the data passed to detail templates.
type DetailPageData struct {
	Title      string
	ActivePage string
	ID         string
}

// ConvoyDetail represents detailed convoy information.
type ConvoyDetail struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Status    string   `json:"status"`
	Progress  string   `json:"progress"`
	Created   string   `json:"created"`
	Issues    []string `json:"issues,omitempty"`
	RawOutput string   `json:"raw_output"`
}

// BeadDetail represents detailed bead/issue information.
type BeadDetail struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Type        string   `json:"type"`
	Priority    int      `json:"priority"`
	Status      string   `json:"status"`
	Owner       string   `json:"owner,omitempty"`
	Assignee    string   `json:"assignee,omitempty"`
	Created     string   `json:"created,omitempty"`
	Updated     string   `json:"updated,omitempty"`
	Description string   `json:"description,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	RawOutput   string   `json:"raw_output"`
}

// handleConvoyDetail serves the convoy detail page.
func (h *GUIHandler) handleConvoyDetail(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path: /convoy/hq-cv-xxx
	id := strings.TrimPrefix(r.URL.Path, "/convoy/")
	if id == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	data := DetailPageData{
		Title:      "Convoy: " + id,
		ActivePage: "",
		ID:         id,
	}
	h.renderTemplate(w, "convoy_detail.html", data)
}

// handleAPIConvoyDetail returns detailed convoy information.
func (h *GUIHandler) handleAPIConvoyDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract ID from path: /api/convoy/hq-cv-xxx
	id := strings.TrimPrefix(r.URL.Path, "/api/convoy/")
	if id == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "missing convoy ID"})
		return
	}

	reader, err := NewBeadsReader("")
	if err == nil {
		if bead, err := reader.GetBead(id); err == nil {
			tracked, _ := reader.GetConvoyTrackedIssues(id)
			completed := 0
			issues := make([]string, 0, len(tracked))
			for _, issue := range tracked {
				issues = append(issues, issue.ID)
				if issue.Status == "closed" {
					completed++
				}
			}

			progress := "0/0 completed"
			if len(tracked) > 0 {
				progress = fmt.Sprintf("%d/%d completed", completed, len(tracked))
			}

			created := ""
			if !bead.CreatedAt.IsZero() {
				created = bead.CreatedAt.Format("2006-01-02")
			}

			json.NewEncoder(w).Encode(ConvoyDetail{
				ID:       bead.ID,
				Title:    bead.Title,
				Status:   bead.Status,
				Progress: progress,
				Created:  created,
				Issues:   issues,
			})
			return
		}
	}

	cmd, cancel := command("gt", "convoy", "status", id)
	defer cancel()
	output, err := cmd.Output()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":         id,
			"error":      err.Error(),
			"raw_output": string(output),
		})
		return
	}

	detail := parseConvoyDetail(id, string(output))
	json.NewEncoder(w).Encode(detail)
}

// parseConvoyDetail parses gt convoy status output.
func parseConvoyDetail(id, output string) ConvoyDetail {
	detail := ConvoyDetail{
		ID:        id,
		RawOutput: output,
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Title line: "🚚 hq-cv-tacik: Test Workflow"
		if strings.HasPrefix(line, "🚚") {
			if colonIdx := strings.Index(line, ":"); colonIdx != -1 {
				rest := strings.TrimSpace(line[colonIdx+1:])
				// Second colon separates ID from title
				if colonIdx2 := strings.Index(rest, ":"); colonIdx2 != -1 {
					detail.Title = strings.TrimSpace(rest[colonIdx2+1:])
				}
			}
		}

		// Status: "  Status:    ●"
		if strings.HasPrefix(line, "Status:") {
			detail.Status = strings.TrimSpace(strings.TrimPrefix(line, "Status:"))
		}

		// Progress: "  Progress:  0/0 completed"
		if strings.HasPrefix(line, "Progress:") {
			detail.Progress = strings.TrimSpace(strings.TrimPrefix(line, "Progress:"))
		}

		// Created: "  Created:   2026-01-17T18:05:17..."
		if strings.HasPrefix(line, "Created:") {
			created := strings.TrimSpace(strings.TrimPrefix(line, "Created:"))
			// Truncate to date only
			if tIdx := strings.Index(created, "T"); tIdx != -1 {
				created = created[:tIdx]
			}
			detail.Created = created
		}
	}

	return detail
}

// handleBeadDetail serves the bead detail page.
func (h *GUIHandler) handleBeadDetail(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path: /bead/te-xxx
	id := strings.TrimPrefix(r.URL.Path, "/bead/")
	if id == "" {
		http.Redirect(w, r, "/workflow", http.StatusFound)
		return
	}

	data := DetailPageData{
		Title:      "Issue: " + id,
		ActivePage: "workflow",
		ID:         id,
	}
	h.renderTemplate(w, "bead_detail.html", data)
}

// handleAPIBeadDetail returns detailed bead information.
func (h *GUIHandler) handleAPIBeadDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract ID from path: /api/bead/te-xxx
	id := strings.TrimPrefix(r.URL.Path, "/api/bead/")
	if id == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "missing bead ID"})
		return
	}

	reader, err := NewBeadsReader("")
	if err == nil {
		if bead, err := reader.GetBead(id); err == nil {
			json.NewEncoder(w).Encode(beadDetailFromBead(bead))
			return
		}
	}

	cmd, cancel := command("bd", "show", id)
	defer cancel()
	output, err := cmd.Output()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":         id,
			"error":      err.Error(),
			"raw_output": string(output),
		})
		return
	}

	detail := parseBeadDetail(id, string(output))
	json.NewEncoder(w).Encode(detail)
}

func beadDetailFromBead(bead *Bead) BeadDetail {
	detail := BeadDetail{
		ID:          bead.ID,
		Title:       bead.Title,
		Type:        bead.Type,
		Priority:    bead.Priority,
		Status:      bead.Status,
		Owner:       bead.Owner,
		Assignee:    bead.Assignee,
		Description: bead.Description,
		Labels:      bead.Labels,
	}

	if !bead.CreatedAt.IsZero() {
		detail.Created = bead.CreatedAt.Format(time.RFC3339)
	}
	if !bead.UpdatedAt.IsZero() {
		detail.Updated = bead.UpdatedAt.Format(time.RFC3339)
	}

	return detail
}

// parseBeadDetail parses bd show output.
func parseBeadDetail(id, output string) BeadDetail {
	detail := BeadDetail{
		ID:        id,
		RawOutput: output,
	}

	lines := strings.Split(output, "\n")
	inDescription := false

	for _, line := range lines {
		// Header line: "? te-0rr [BUG] · SSH server accepts..."
		if strings.HasPrefix(line, "?") || strings.HasPrefix(line, "●") || strings.HasPrefix(line, "✓") {
			// Extract status indicator
			if strings.HasPrefix(line, "?") {
				detail.Status = "open"
			} else if strings.HasPrefix(line, "●") {
				detail.Status = "in_progress"
			} else if strings.HasPrefix(line, "✓") {
				detail.Status = "closed"
			}

			// Extract type: [BUG], [TASK], [FEATURE], [DOC]
			if strings.Contains(line, "[BUG]") {
				detail.Type = "bug"
			} else if strings.Contains(line, "[TASK]") {
				detail.Type = "task"
			} else if strings.Contains(line, "[FEATURE]") {
				detail.Type = "feature"
			} else if strings.Contains(line, "[DOC]") {
				detail.Type = "doc"
			}

			// Extract priority: [● P1], [● P2], etc
			for i := 1; i <= 4; i++ {
				if strings.Contains(line, "P"+string(rune('0'+i))) {
					detail.Priority = i
					break
				}
			}

			// Extract title: after the ID and brackets
			if dotIdx := strings.Index(line, "·"); dotIdx != -1 {
				rest := line[dotIdx+1:]
				if dotIdx2 := strings.Index(rest, "·"); dotIdx2 != -1 {
					detail.Title = strings.TrimSpace(rest[:dotIdx2])
				} else {
					// Handle case without second dot
					rest = strings.TrimSpace(rest)
					if bracketIdx := strings.Index(rest, "["); bracketIdx != -1 {
						detail.Title = strings.TrimSpace(rest[:bracketIdx])
					} else {
						detail.Title = rest
					}
				}
			}
		}

		// Owner/Assignee line: "Owner: mayor · Assignee: ..."
		if strings.HasPrefix(line, "Owner:") {
			parts := strings.Split(line, "·")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, "Owner:") {
					detail.Owner = strings.TrimSpace(strings.TrimPrefix(part, "Owner:"))
				}
				if strings.HasPrefix(part, "Assignee:") {
					detail.Assignee = strings.TrimSpace(strings.TrimPrefix(part, "Assignee:"))
				}
			}
		}

		// Created/Updated line: "Created: 2026-01-17 · Updated: 2026-01-17"
		if strings.HasPrefix(line, "Created:") {
			parts := strings.Split(line, "·")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, "Created:") {
					detail.Created = strings.TrimSpace(strings.TrimPrefix(part, "Created:"))
				}
				if strings.HasPrefix(part, "Updated:") {
					detail.Updated = strings.TrimSpace(strings.TrimPrefix(part, "Updated:"))
				}
			}
		}

		// Description section
		if line == "DESCRIPTION" {
			inDescription = true
			continue
		}

		// Labels section ends description
		if strings.HasPrefix(line, "LABELS:") {
			inDescription = false
			labelsStr := strings.TrimSpace(strings.TrimPrefix(line, "LABELS:"))
			if labelsStr != "" {
				detail.Labels = strings.Split(labelsStr, ", ")
			}
		}

		// Collect description lines
		if inDescription && strings.TrimSpace(line) != "" {
			if detail.Description != "" {
				detail.Description += "\n"
			}
			detail.Description += line
		}
	}

	return detail
}
