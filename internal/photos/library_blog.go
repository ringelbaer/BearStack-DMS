// Datei liest Blogbeitraege aus dem Fotoverzeichnis und verbindet sie mit dem Fotoindex.
package photos

import (
	"html"
	"html/template"
	"os"
	"strings"
	"time"
)

func (l *Library) blogFromPath(rel string) (BlogPost, error) {
	post, err := l.blogFromPathData(rel)
	if err != nil {
		return BlogPost{}, err
	}
	l.saveBlog(post)
	return post, nil
}

func (l *Library) blogFromPathData(rel string) (BlogPost, error) {
	abs, err := l.Resolve(rel)
	if err != nil {
		return BlogPost{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return BlogPost{}, err
	}
	adminOnly := l.directoryAdminOnly(parentPath(rel))
	post, _, err := l.blogFromPathInfo(rel, abs, info, nil, adminOnly)
	if err != nil {
		return BlogPost{}, err
	}
	return post, nil
}

func markdownDate(raw []byte) *time.Time {
	lines := strings.Split(string(raw), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "<!-- @pg-date ") || !strings.HasSuffix(line, " -->") {
			continue
		}
		value := strings.TrimSuffix(strings.TrimPrefix(line, "<!-- @pg-date "), " -->")
		if parsed, err := time.Parse("2006-01-02", strings.TrimSpace(value)); err == nil {
			return &parsed
		}
	}
	return nil
}

func renderMarkdown(raw []byte) template.HTML {
	var b strings.Builder
	inList := false
	lines := strings.Split(string(raw), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "<!-- @pg-date ") {
			continue
		}
		if trimmed == "" {
			if inList {
				b.WriteString("</ul>")
				inList = false
			}
			continue
		}
		if strings.HasPrefix(trimmed, "### ") {
			if inList {
				b.WriteString("</ul>")
				inList = false
			}
			b.WriteString("<h4>" + html.EscapeString(strings.TrimSpace(strings.TrimPrefix(trimmed, "### "))) + "</h4>")
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			if inList {
				b.WriteString("</ul>")
				inList = false
			}
			b.WriteString("<h3>" + html.EscapeString(strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))) + "</h3>")
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			if inList {
				b.WriteString("</ul>")
				inList = false
			}
			b.WriteString("<h2>" + html.EscapeString(strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))) + "</h2>")
			continue
		}
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			if !inList {
				b.WriteString("<ul>")
				inList = true
			}
			b.WriteString("<li>" + html.EscapeString(strings.TrimSpace(trimmed[2:])) + "</li>")
			continue
		}
		if inList {
			b.WriteString("</ul>")
			inList = false
		}
		b.WriteString("<p>" + html.EscapeString(trimmed) + "</p>")
	}
	if inList {
		b.WriteString("</ul>")
	}
	return template.HTML(b.String())
}

func markdownText(raw []byte) string {
	lines := strings.Split(string(raw), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "<!-- @pg-date ") {
			continue
		}
		line = strings.TrimPrefix(line, "### ")
		line = strings.TrimPrefix(line, "## ")
		line = strings.TrimPrefix(line, "# ")
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}
