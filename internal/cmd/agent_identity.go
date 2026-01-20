package cmd

import "github.com/steveyegge/gastown/internal/beads"

func normalizeTownIdentity(identity string) string {
	switch identity {
	case "mayor", "mayor/":
		return "mayor/"
	case "deacon", "deacon/":
		return "deacon/"
	default:
		return identity
	}
}

func assigneeCandidates(identity string) []string {
	normalized := normalizeTownIdentity(identity)
	switch normalized {
	case "mayor/":
		return []string{"mayor/", "mayor"}
	case "deacon/":
		return []string{"deacon/", "deacon"}
	default:
		return []string{normalized}
	}
}

func listBeadsForAssignee(b *beads.Beads, opts beads.ListOptions) ([]*beads.Issue, error) {
	if opts.Assignee == "" || opts.NoAssignee {
		return b.List(opts)
	}

	candidates := assigneeCandidates(opts.Assignee)
	seen := make(map[string]bool, len(candidates))
	var results []*beads.Issue

	for _, assignee := range candidates {
		opts.Assignee = assignee
		issues, err := b.List(opts)
		if err != nil {
			return nil, err
		}
		for _, issue := range issues {
			if issue == nil || seen[issue.ID] {
				continue
			}
			seen[issue.ID] = true
			results = append(results, issue)
		}
	}

	return results, nil
}
