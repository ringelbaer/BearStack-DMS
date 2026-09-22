package photos

import (
	"strings"
	"time"
)

type PersonSummaryPart struct {
	Text     string
	PersonID int64
}

// SummaryParts keeps person identities separate from display text, so views can
// link visible relations without parsing names or inserting trusted HTML.
// Divorced marriages remain in the master record, not the compact heading.
func (d PersonDetails) SummaryParts() []PersonSummaryPart {
	var parts []PersonSummaryPart
	field := func(label string, values ...PersonSummaryPart) {
		if len(parts) > 0 {
			parts = append(parts, PersonSummaryPart{Text: " · "})
		}
		parts = append(parts, PersonSummaryPart{Text: label + ": "})
		parts = append(parts, values...)
	}
	date := func(value string) string {
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil {
			return value
		}
		return parsed.Format("02.01.2006")
	}
	if d.BirthDate != "" {
		field("Geboren", PersonSummaryPart{Text: date(d.BirthDate)})
	}
	if d.DeathDate != "" {
		field("Gestorben", PersonSummaryPart{Text: date(d.DeathDate)})
	}
	if d.Parents.Mother != nil {
		field("Mutter", PersonSummaryPart{Text: d.Parents.Mother.Name, PersonID: d.Parents.Mother.ID})
	}
	if d.Parents.Father != nil {
		field("Vater", PersonSummaryPart{Text: d.Parents.Father.Name, PersonID: d.Parents.Father.ID})
	}
	if len(d.Siblings) > 0 {
		var names []PersonSummaryPart
		for i, p := range d.Siblings {
			if i > 0 {
				names = append(names, PersonSummaryPart{Text: ", "})
			}
			names = append(names, PersonSummaryPart{Text: p.Name, PersonID: p.ID})
		}
		field("Geschwister", names...)
	}
	for _, m := range d.Marriages {
		if m.DivorceDate != "" {
			continue
		}
		field("Verheiratet mit", PersonSummaryPart{Text: m.Spouse.Name, PersonID: m.Spouse.ID})
		if m.WeddingDate != "" {
			parts = append(parts, PersonSummaryPart{Text: " seit " + date(m.WeddingDate)})
		}
	}
	return parts
}

func (d PersonDetails) Summary() string {
	var summary strings.Builder
	for _, part := range d.SummaryParts() {
		summary.WriteString(part.Text)
	}
	return summary.String()
}
