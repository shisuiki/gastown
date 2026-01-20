package protocol

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/mail"
)

// DefaultRefineryHandler provides the default implementation for Refinery protocol handlers.
// It receives MERGE_READY messages from the Witness and acknowledges verified work.
// Note: The Refinery now queries beads directly for merge requests (via ReadyWithType).
type DefaultRefineryHandler struct {
	// Rig is the name of the rig this refinery processes.
	Rig string

	// WorkDir is the working directory for operations.
	WorkDir string

	// Router is used to send mail messages.
	Router *mail.Router

	// Output is where to write status messages.
	Output io.Writer
}

// NewRefineryHandler creates a new DefaultRefineryHandler.
func NewRefineryHandler(rig, workDir string) *DefaultRefineryHandler {
	return &DefaultRefineryHandler{
		Rig:     rig,
		WorkDir: workDir,
		Router:  mail.NewRouter(workDir),
		Output:  os.Stdout,
	}
}

// SetOutput sets the output writer for status messages.
func (h *DefaultRefineryHandler) SetOutput(w io.Writer) {
	h.Output = w
}

// HandleMergeReady handles a MERGE_READY message from Witness.
// When a polecat's work is verified and ready, the Refinery acknowledges receipt.
//
// NOTE: The merge-request bead is created by `gt done`, so we no longer need
// to add to the mrqueue here. The Refinery queries beads directly for ready MRs.
func (h *DefaultRefineryHandler) HandleMergeReady(payload *MergeReadyPayload) error {
	issue := normalizeMergeReadyIssue(payload.Issue)
	mrID := ""

	bd := beads.New(h.WorkDir)
	mr, err := bd.FindMRForBranch(payload.Branch)
	if err != nil {
		return fmt.Errorf("lookup MR for branch %s: %w", payload.Branch, err)
	}

	if mr != nil {
		mrID = mr.ID
		if fields := beads.ParseMRFields(mr); fields != nil && fields.SourceIssue != "" {
			issue = fields.SourceIssue
		}
	}

	if issue == "" {
		issue = inferIssueFromBranch(payload.Branch)
	}

	payload.Issue = issue

	_, _ = fmt.Fprintf(h.Output, "[Refinery] MERGE_READY received for polecat %s\n", payload.Polecat)
	_, _ = fmt.Fprintf(h.Output, "  Branch: %s\n", payload.Branch)
	_, _ = fmt.Fprintf(h.Output, "  Issue: %s\n", payload.Issue)
	if mrID != "" {
		_, _ = fmt.Fprintf(h.Output, "  MR: %s\n", mrID)
	}
	_, _ = fmt.Fprintf(h.Output, "  Verified: %s\n", payload.Verified)

	// Validate required fields
	if payload.Branch == "" {
		return fmt.Errorf("missing branch in MERGE_READY payload")
	}
	if payload.Polecat == "" {
		return fmt.Errorf("missing polecat in MERGE_READY payload")
	}
	if payload.Issue == "" || strings.EqualFold(payload.Issue, "none") {
		reason := "missing issue in MERGE_READY payload"
		if mr == nil {
			reason = "missing issue and no merge-request bead found for branch"
		}
		if err := h.notifyInvalidMergeReady(payload, reason, mrID); err != nil {
			return fmt.Errorf("sending merge-ready failure notice: %w", err)
		}
		return fmt.Errorf("invalid MERGE_READY: %s (branch=%s)", reason, payload.Branch)
	}
	if mr == nil {
		reason := "no merge-request bead found for branch"
		if err := h.notifyInvalidMergeReady(payload, reason, mrID); err != nil {
			return fmt.Errorf("sending merge-ready failure notice: %w", err)
		}
		return fmt.Errorf("invalid MERGE_READY: %s (branch=%s)", reason, payload.Branch)
	}

	// The merge-request bead is created by `gt done` with gt:merge-request label.
	// The Refinery queries beads directly via ReadyWithType("merge-request").
	// No need to add to mrqueue - that was a duplicate tracking file.
	_, _ = fmt.Fprintf(h.Output, "[Refinery] ✓ Work verified - Refinery will pick up MR via beads query\n")

	return nil
}

// SendMerged sends a MERGED message to the Witness.
// Called by the Refinery after successfully merging a branch.
func (h *DefaultRefineryHandler) SendMerged(polecat, branch, issue, targetBranch, mergeCommit string) error {
	msg := NewMergedMessage(h.Rig, polecat, branch, issue, targetBranch, mergeCommit)
	return h.Router.Send(msg)
}

// SendMergeFailed sends a MERGE_FAILED message to the Witness.
// Called by the Refinery when a merge fails.
func (h *DefaultRefineryHandler) SendMergeFailed(polecat, branch, issue, targetBranch, failureType, errorMsg string) error {
	msg := NewMergeFailedMessage(h.Rig, polecat, branch, issue, targetBranch, failureType, errorMsg)
	return h.Router.Send(msg)
}

// SendReworkRequest sends a REWORK_REQUEST message to the Witness.
// Called by the Refinery when a branch has conflicts.
func (h *DefaultRefineryHandler) SendReworkRequest(polecat, branch, issue, targetBranch string, conflictFiles []string) error {
	msg := NewReworkRequestMessage(h.Rig, polecat, branch, issue, targetBranch, conflictFiles)
	return h.Router.Send(msg)
}

// NotifyMergeOutcome is a convenience method that sends the appropriate message
// based on the merge result.
type MergeOutcome struct {
	// Success indicates whether the merge was successful.
	Success bool

	// Conflict indicates the failure was due to conflicts (needs rebase).
	Conflict bool

	// FailureType categorizes the failure (e.g., "tests", "build").
	FailureType string

	// Error is the error message if the merge failed.
	Error string

	// MergeCommit is the SHA of the merge commit on success.
	MergeCommit string

	// ConflictFiles lists files with conflicts (if Conflict is true).
	ConflictFiles []string
}

// NotifyMergeOutcome sends the appropriate protocol message based on the outcome.
func (h *DefaultRefineryHandler) NotifyMergeOutcome(polecat, branch, issue, targetBranch string, outcome MergeOutcome) error {
	if outcome.Success {
		return h.SendMerged(polecat, branch, issue, targetBranch, outcome.MergeCommit)
	}

	if outcome.Conflict {
		return h.SendReworkRequest(polecat, branch, issue, targetBranch, outcome.ConflictFiles)
	}

	return h.SendMergeFailed(polecat, branch, issue, targetBranch, outcome.FailureType, outcome.Error)
}

// Ensure DefaultRefineryHandler implements RefineryHandler.
var _ RefineryHandler = (*DefaultRefineryHandler)(nil)

var mergeReadyIssuePattern = regexp.MustCompile(`^[a-z]+-[a-z0-9]+(?:\.[0-9]+)?$`)

func normalizeMergeReadyIssue(issue string) string {
	issue = strings.TrimSpace(issue)
	if strings.EqualFold(issue, "none") {
		return ""
	}
	return issue
}

func inferIssueFromBranch(branch string) string {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return ""
	}

	if strings.HasPrefix(branch, "polecat/") {
		parts := strings.Split(branch, "/")
		if len(parts) >= 3 {
			issue := strings.TrimSpace(parts[2])
			if at := strings.Index(issue, "@"); at > 0 {
				issue = issue[:at]
			}
			if mergeReadyIssuePattern.MatchString(issue) {
				return issue
			}
		}
		return ""
	}

	for _, part := range strings.Split(branch, "/") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if at := strings.Index(part, "@"); at > 0 {
			part = part[:at]
		}
		if mergeReadyIssuePattern.MatchString(part) {
			return part
		}
	}

	return ""
}

func (h *DefaultRefineryHandler) notifyInvalidMergeReady(payload *MergeReadyPayload, reason, mrID string) error {
	msg := &mail.Message{
		From:     fmt.Sprintf("%s/refinery", h.Rig),
		To:       fmt.Sprintf("%s/witness", h.Rig),
		Subject:  fmt.Sprintf("MERGE_READY invalid %s", payload.Polecat),
		Priority: mail.PriorityHigh,
		Type:     mail.TypeTask,
		Body: fmt.Sprintf(`MERGE_READY could not be processed.

Branch: %s
Issue: %s
Polecat: %s
MR: %s
Reason: %s

Action: ensure a merge-request bead exists (run "gt done" or
"gt mq submit --issue <id>") and resend MERGE_READY with a valid Issue.`,
			payload.Branch,
			payload.Issue,
			payload.Polecat,
			emptyIfUnset(mrID),
			reason,
		),
	}

	return h.Router.Send(msg)
}

func emptyIfUnset(value string) string {
	if value == "" {
		return "none"
	}
	return value
}
