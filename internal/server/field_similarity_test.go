package server

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	"bearstack/internal/document"
)

func TestLevenshteinWithinMatchesFullDistance(t *testing.T) {
	words := []string{""}
	for length, start := 1, 0; length <= 4; length++ {
		end := len(words)
		for _, prefix := range words[start:end] {
			for _, letter := range []string{"a", "b", "ä"} {
				words = append(words, prefix+letter)
			}
		}
		start = end
	}
	random := rand.New(rand.NewSource(7))
	for i := 0; i < 100; i++ {
		word := make([]rune, random.Intn(30))
		for j := range word {
			word[j] = []rune("abä漢")[random.Intn(4)]
		}
		words = append(words, string(word))
	}
	prev, curr := make([]int, 64), make([]int, 64)
	for _, a := range words {
		for _, b := range words {
			distance := referenceLevenshtein([]rune(a), []rune(b))
			for limit := 0; limit <= 2; limit++ {
				if got := levenshteinWithin([]rune(a), []rune(b), limit, prev, curr); got != (distance <= limit) {
					t.Fatalf("%q / %q: within %d = %v, distance=%d", a, b, limit, got, distance)
				}
			}
		}
	}
}

// Independent full-matrix oracle, including distances above the cutoff.
func referenceLevenshtein(a, b []rune) int {
	matrix := make([][]int, len(a)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(b)+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			matrix[i][j] = min(matrix[i-1][j]+1, matrix[i][j-1]+1, matrix[i-1][j-1]+cost)
		}
	}
	return matrix[len(a)][len(b)]
}

func TestSimilarCustomFieldValuesPreservesNormalizationAndUnicodeThreshold(t *testing.T) {
	values := []document.CustomFieldValue{{Value: "A.B"}, {Value: " ab "}, {Value: "ac"}, {Value: "漢字"}, {Value: "文字"}, {Value: "文法"}, {Value: "---"}}
	want := []document.CustomFieldValueSuggestion{
		{Value: " ab ", Similar: []string{"A.B", "ac"}, Reason: "Unterschied nur bei Großschreibung, Leerzeichen oder Satzzeichen"},
		{Value: " ab ", Similar: []string{"ac"}, Reason: "Sehr ähnliche Schreibweise"},
		{Value: "文字", Similar: []string{"文法", "漢字"}, Reason: "Sehr ähnliche Schreibweise"},
		{Value: "文字", Similar: []string{"文法"}, Reason: "Sehr ähnliche Schreibweise"},
	}
	if got := similarCustomFieldValues(values); !reflect.DeepEqual(got, want) {
		t.Fatalf("suggestions = %#v, want %#v", got, want)
	}
}

func BenchmarkSimilarCustomFieldValues(b *testing.B) {
	values := make([]document.CustomFieldValue, 1500)
	for i := range values {
		values[i].Value = fmt.Sprintf("Firma %08x", uint32(i)*2654435761)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		similarCustomFieldValues(values)
	}
}
