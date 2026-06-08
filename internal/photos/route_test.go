package photos

import "testing"

func TestNormalizeRouteClusterRadiusMeters(t *testing.T) {
	tests := []struct {
		name  string
		value int
		want  int
	}{
		{name: "default", value: 0, want: DefaultRouteClusterRadiusMeters},
		{name: "minimum", value: 42, want: 500},
		{name: "nearest low", value: 1300, want: 1500},
		{name: "exact option", value: 3000, want: 3000},
		{name: "nearest high", value: 6200, want: 5000},
		{name: "maximum", value: 50000, want: 10000},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := NormalizeRouteClusterRadiusMeters(test.value); got != test.want {
				t.Fatalf("NormalizeRouteClusterRadiusMeters(%d) = %d, want %d", test.value, got, test.want)
			}
		})
	}
}
