package tagutil

import "testing"

func TestNormalize(t *testing.T) {
	tags := Normalize([]string{" Rechnung ", "steuer", "Rechnung", "  zwei   worte  ", ""})
	want := []string{"rechnung", "steuer", "zwei worte"}
	if len(tags) != len(want) {
		t.Fatalf("tags = %#v", tags)
	}
	for i := range want {
		if tags[i] != want[i] {
			t.Fatalf("tags[%d] = %q, want %q", i, tags[i], want[i])
		}
	}
}

func TestNormalizeString(t *testing.T) {
	tags := NormalizeString("Rechnung, steuer; Rechnung\nPrivat")
	want := []string{"rechnung", "steuer", "privat"}
	if len(tags) != len(want) {
		t.Fatalf("tags = %#v", tags)
	}
	for i := range want {
		if tags[i] != want[i] {
			t.Fatalf("tags[%d] = %q, want %q", i, tags[i], want[i])
		}
	}
}

func TestMergeRemoveAndEqualNormalized(t *testing.T) {
	merged := Merge([]string{" Steuer ", "arbeit"}, []string{"steuer", "Neu"})
	wantMerged := []string{"steuer", "arbeit", "neu"}
	if len(merged) != len(wantMerged) {
		t.Fatalf("merged = %#v", merged)
	}
	for i := range wantMerged {
		if merged[i] != wantMerged[i] {
			t.Fatalf("merged[%d] = %q, want %q", i, merged[i], wantMerged[i])
		}
	}
	removed := Remove(merged, []string{" Arbeit "})
	if len(removed) != 2 || removed[0] != "steuer" || removed[1] != "neu" {
		t.Fatalf("removed = %#v", removed)
	}
	if !EqualNormalized([]string{"Steuer", "neu"}, removed) {
		t.Fatalf("EqualNormalized did not match %#v", removed)
	}
}

func TestDisplayName(t *testing.T) {
	cases := []struct {
		mode string
		tag  string
		want string
	}{
		{mode: DisplayModeLower, tag: "Steuer", want: "steuer"},
		{mode: DisplayModeUpper, tag: "steuer", want: "STEUER"},
		{mode: DisplayModeFirst, tag: "sTEUER", want: "Steuer"},
		{mode: "bad", tag: "Steuer", want: "steuer"},
	}
	for _, tc := range cases {
		if got := DisplayName(tc.mode, tc.tag); got != tc.want {
			t.Fatalf("DisplayName(%q, %q) = %q, want %q", tc.mode, tc.tag, got, tc.want)
		}
	}
}

func TestNormalizeColor(t *testing.T) {
	cases := []struct {
		name  string
		color string
		want  string
	}{
		{name: "lowercases valid color", color: " #AA00CC ", want: "#aa00cc"},
		{name: "rejects missing hash", color: "aa00cc", want: DefaultColor},
		{name: "rejects invalid digit", color: "#gg00cc", want: DefaultColor},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeColor(tc.color); got != tc.want {
				t.Fatalf("NormalizeColor(%q) = %q, want %q", tc.color, got, tc.want)
			}
		})
	}
}

func TestNormalizeColorOr(t *testing.T) {
	if got := NormalizeColorOr("bad", "#112233"); got != "#112233" {
		t.Fatalf("fallback color = %q", got)
	}
	if got := NormalizeColorOr("bad", "also-bad"); got != DefaultColor {
		t.Fatalf("invalid fallback color = %q", got)
	}
}

func TestReadableTextColor(t *testing.T) {
	if got := ReadableTextColor("#ffffff"); got != "#172026" {
		t.Fatalf("light text color = %q", got)
	}
	if got := ReadableTextColor("#000000"); got != "#ffffff" {
		t.Fatalf("dark text color = %q", got)
	}
}
