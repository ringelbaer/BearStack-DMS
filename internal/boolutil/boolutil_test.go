package boolutil

import "testing"

func TestParse(t *testing.T) {
	for _, value := range []string{"1", "true", "TRUE", " yes ", "on"} {
		got, ok := Parse(value)
		if !ok || !got {
			t.Fatalf("Parse(%q) = %v, %v, want true, true", value, got, ok)
		}
	}
	for _, value := range []string{"0", "false", "FALSE", " no ", "off"} {
		got, ok := Parse(value)
		if !ok || got {
			t.Fatalf("Parse(%q) = %v, %v, want false, true", value, got, ok)
		}
	}
	if got, ok := Parse("maybe"); ok || got {
		t.Fatalf("Parse unknown = %v, %v, want false, false", got, ok)
	}
}
