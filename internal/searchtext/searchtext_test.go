package searchtext

import (
	"slices"
	"testing"
)

func TestGermanFold(t *testing.T) {
	if got := GermanFold("März Öl Übung Straße"); got != "maerz oel uebung strasse" {
		t.Fatalf("fold = %q", got)
	}
}

func TestFTSAndQueryUsesGermanVariantsAndQuotes(t *testing.T) {
	got := FTSAndQuery(`März Bericht`, 16)
	want := `("märz" OR "maerz") AND "bericht"`
	if got != want {
		t.Fatalf("fts query = %q, want %q", got, want)
	}
}

func TestFTSAndQueryRejectsShortTerms(t *testing.T) {
	if got := FTSAndQuery(`ab Steuer`, 16); got != "" {
		t.Fatalf("short-term fts query = %q, want empty fallback", got)
	}
}

func TestFTSLiteralEscapesQuotes(t *testing.T) {
	if got := FTSLiteral(`a"b`); got != `"a""b"` {
		t.Fatalf("literal = %q", got)
	}
}

func TestLikeContainsPatternEscapesSQLWildcards(t *testing.T) {
	if got := LikeContainsPattern(`a_b%c*\d`); got != `%a\_b\%c%\\d%` {
		t.Fatalf("pattern = %q", got)
	}
}

func TestGermanVariants(t *testing.T) {
	variants := GermanVariants("Maerz")
	for _, want := range []string{"maerz", "märz"} {
		if !slices.Contains(variants, want) {
			t.Fatalf("variants %v miss %q", variants, want)
		}
	}
	variants = GermanVariants("ä")
	for _, want := range []string{"ä", "ae"} {
		if !slices.Contains(variants, want) {
			t.Fatalf("variants %v miss %q", variants, want)
		}
	}
}
