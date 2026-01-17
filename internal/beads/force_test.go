package beads

import "testing"

func TestNeedsForceForID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{
			name: "single hyphen - no force needed",
			id:   "gt-mayor",
			want: false,
		},
		{
			name: "two hyphens - force needed",
			id:   "gt-gastown-witness",
			want: true,
		},
		{
			name: "three hyphens - force needed",
			id:   "gt-gastown-polecat-Toast",
			want: true,
		},
		{
			name: "four hyphens - force needed",
			id:   "test-testrig-polecat-tombstone",
			want: true,
		},
		{
			name: "convoy ID with hyphens",
			id:   "hq-cv-abc123",
			want: true,
		},
		{
			name: "no hyphens - no force needed",
			id:   "test",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NeedsForceForID(tt.id)
			if got != tt.want {
				t.Errorf("NeedsForceForID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}
