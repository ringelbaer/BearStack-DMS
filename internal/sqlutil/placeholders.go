// Datei stellt SQL-Helfer wie Platzhalterlisten fuer dynamisch gebaute Statements bereit.
package sqlutil

import (
	"strings"
	"sync"
)

var placeholderCache sync.Map

func Placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	if cached, ok := placeholderCache.Load(count); ok {
		return cached.(string)
	}
	var b strings.Builder
	b.Grow(count*3 - 2)
	for i := 0; i < count; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteByte('?')
	}
	sqlText := b.String()
	actual, _ := placeholderCache.LoadOrStore(count, sqlText)
	return actual.(string)
}
