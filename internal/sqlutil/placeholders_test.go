package sqlutil

import "testing"

func TestPlaceholders(t *testing.T) {
	tests := []struct {
		name  string
		count int
		want  string
	}{
		{name: "zero", count: 0, want: ""},
		{name: "one", count: 1, want: "?"},
		{name: "many", count: 3, want: "?, ?, ?"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Placeholders(tt.count); got != tt.want {
				t.Fatalf("Placeholders(%d) = %q, want %q", tt.count, got, tt.want)
			}
		})
	}
}
