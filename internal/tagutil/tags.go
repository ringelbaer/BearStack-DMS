// Datei normalisiert Tags, Farben und Tag-Listen fuer Dokumente und Fotos.
package tagutil

import (
	"strconv"
	"strings"
)

const DefaultColor = "#176b87"

const (
	DisplayModeLower = "strtolower"
	DisplayModeUpper = "strtoupper"
	DisplayModeFirst = "ucfirst"
)

func Normalize(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	tags := make([]string, 0, len(values))
	for _, value := range values {
		tag := strings.ToLower(strings.TrimSpace(value))
		tag = strings.Join(strings.Fields(tag), " ")
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	return tags
}

func NormalizeString(input string) []string {
	parts := strings.FieldsFunc(input, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n'
	})
	return Normalize(parts)
}

func Merge(existing, add []string) []string {
	values := make([]string, 0, len(existing)+len(add))
	values = append(values, existing...)
	values = append(values, add...)
	return Normalize(values)
}

func Remove(existing, remove []string) []string {
	removeSet := map[string]struct{}{}
	for _, tag := range Normalize(remove) {
		removeSet[tag] = struct{}{}
	}
	current := Normalize(existing)
	out := make([]string, 0, len(current))
	for _, tag := range current {
		if _, ok := removeSet[tag]; ok {
			continue
		}
		out = append(out, tag)
	}
	return out
}

func EqualNormalized(left, right []string) bool {
	left = Normalize(left)
	right = Normalize(right)
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func NormalizeDisplayMode(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case DisplayModeUpper:
		return DisplayModeUpper
	case DisplayModeFirst:
		return DisplayModeFirst
	default:
		return DisplayModeLower
	}
}

func DisplayName(mode, tag string) string {
	tag = strings.TrimSpace(tag)
	switch NormalizeDisplayMode(mode) {
	case DisplayModeUpper:
		return strings.ToUpper(tag)
	case DisplayModeFirst:
		lower := strings.ToLower(tag)
		runes := []rune(lower)
		if len(runes) == 0 {
			return ""
		}
		runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
		return string(runes)
	default:
		return strings.ToLower(tag)
	}
}

func NormalizeColor(color string) string {
	return NormalizeColorOr(color, DefaultColor)
}

func NormalizeColorOr(color, fallback string) string {
	fallback = normalizeFallbackColor(fallback)
	color = strings.TrimSpace(strings.ToLower(color))
	if len(color) != 7 || color[0] != '#' {
		return fallback
	}
	for _, r := range color[1:] {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return fallback
		}
	}
	return color
}

func ReadableTextColor(color string) string {
	color = NormalizeColor(color)
	r, _ := strconv.ParseInt(color[1:3], 16, 64)
	g, _ := strconv.ParseInt(color[3:5], 16, 64)
	b, _ := strconv.ParseInt(color[5:7], 16, 64)
	luminance := 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
	if luminance > 150 {
		return "#172026"
	}
	return "#ffffff"
}

func normalizeFallbackColor(fallback string) string {
	fallback = strings.TrimSpace(strings.ToLower(fallback))
	if len(fallback) == 7 && fallback[0] == '#' {
		for _, r := range fallback[1:] {
			if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
				return DefaultColor
			}
		}
		return fallback
	}
	return DefaultColor
}
