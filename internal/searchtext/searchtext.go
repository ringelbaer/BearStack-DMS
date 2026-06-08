// Datei normalisiert Suchtexte und stellt gemeinsame Helfer fuer Volltext-Indexierung bereit.
package searchtext

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxGermanVariants = 32

func FTSTokens(input string, max int) []string {
	tokens := strings.FieldsFunc(strings.ToLower(input), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	terms := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		terms = append(terms, token)
		if max > 0 && len(terms) == max {
			break
		}
	}
	return terms
}

func FTSAndQuery(input string, maxTokens int) string {
	tokens := FTSTokens(input, maxTokens)
	terms := make([]string, 0, len(tokens))
	for _, token := range tokens {
		fts, ok := FTSQueryTerm(token)
		if !ok {
			return ""
		}
		if fts != "" {
			terms = append(terms, fts)
		}
	}
	return strings.Join(terms, " AND ")
}

func FTSQueryTerm(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", true
	}
	variants := GermanVariants(value)
	if len(variants) == 0 {
		return "", true
	}
	terms := make([]string, 0, len(variants))
	for _, variant := range variants {
		if RuneLen(variant) < 3 {
			return "", false
		}
		terms = append(terms, FTSLiteral(variant))
	}
	if len(terms) == 1 {
		return terms[0], true
	}
	return "(" + strings.Join(terms, " OR ") + ")", true
}

func FTSLiteral(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, `"`, `""`)
	return `"` + value + `"`
}

func LikeContainsPattern(value string) string {
	var b strings.Builder
	b.WriteByte('%')
	for _, r := range value {
		switch r {
		case '*':
			b.WriteByte('%')
		case '\\', '%', '_':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('%')
	return b.String()
}

func GermanFold(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		switch unicode.ToLower(r) {
		case 'ä':
			b.WriteString("ae")
		case 'ö':
			b.WriteString("oe")
		case 'ü':
			b.WriteString("ue")
		case 'ß':
			b.WriteString("ss")
		default:
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

func GermanVariants(value string) []string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return nil
	}
	seen := map[string]struct{}{}
	var variants []string
	var walk func(string, string)
	walk = func(rest, prefix string) {
		if len(variants) >= maxGermanVariants {
			return
		}
		if rest == "" {
			if _, ok := seen[prefix]; !ok {
				seen[prefix] = struct{}{}
				variants = append(variants, prefix)
			}
			return
		}
		switch {
		case strings.HasPrefix(rest, "ae"):
			walk(rest[2:], prefix+"ae")
			walk(rest[2:], prefix+"ä")
			return
		case strings.HasPrefix(rest, "oe"):
			walk(rest[2:], prefix+"oe")
			walk(rest[2:], prefix+"ö")
			return
		case strings.HasPrefix(rest, "ue"):
			walk(rest[2:], prefix+"ue")
			walk(rest[2:], prefix+"ü")
			return
		case strings.HasPrefix(rest, "ss"):
			walk(rest[2:], prefix+"ss")
			walk(rest[2:], prefix+"ß")
			return
		}
		r, size := utf8.DecodeRuneInString(rest)
		next := rest[size:]
		switch r {
		case 'ä':
			walk(next, prefix+"ä")
			walk(next, prefix+"ae")
		case 'ö':
			walk(next, prefix+"ö")
			walk(next, prefix+"oe")
		case 'ü':
			walk(next, prefix+"ü")
			walk(next, prefix+"ue")
		case 'ß':
			walk(next, prefix+"ß")
			walk(next, prefix+"ss")
		default:
			walk(next, prefix+string(r))
		}
	}
	walk(value, "")
	folded := GermanFold(value)
	if _, ok := seen[folded]; !ok && len(variants) < maxGermanVariants {
		variants = append(variants, folded)
	}
	return variants
}

func RuneLen(value string) int {
	return utf8.RuneCountInString(value)
}
