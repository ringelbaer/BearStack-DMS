package uploadlimit

import "testing"

func TestEnvelopeLimit(t *testing.T) {
	tests := []struct {
		name string
		max  int64
		want int64
	}{
		{name: "default", max: 0, want: DefaultMaxBytes},
		{name: "configured", max: 64, want: 64 * EnvelopeFactor},
		{name: "overflow guard", max: (1<<62)/EnvelopeFactor + 1, want: (1<<62)/EnvelopeFactor + 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EnvelopeLimit(tt.max); got != tt.want {
				t.Fatalf("EnvelopeLimit(%d) = %d, want %d", tt.max, got, tt.want)
			}
		})
	}
}
