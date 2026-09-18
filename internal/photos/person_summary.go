package photos

import (
	"strings"
	"time"
)

// Summary contains only recorded, visible details. Divorced marriages remain in
// the editable master record but are omitted from the compact person heading.
func (d PersonDetails) Summary() string {
	parts := []string{}
	date := func(value string) string {
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil {
			return value
		}
		return parsed.Format("02.01.2006")
	}
	if d.BirthDate != "" {
		parts = append(parts, "Geboren: "+date(d.BirthDate))
	}
	if d.DeathDate != "" {
		parts = append(parts, "Gestorben: "+date(d.DeathDate))
	}
	if d.Parents.Mother != nil {
		parts = append(parts, "Mutter: "+d.Parents.Mother.Name)
	}
	if d.Parents.Father != nil {
		parts = append(parts, "Vater: "+d.Parents.Father.Name)
	}
	if len(d.Siblings) > 0 {
		names := []string{}
		for _, p := range d.Siblings {
			names = append(names, p.Name)
		}
		parts = append(parts, "Geschwister: "+strings.Join(names, ", "))
	}
	for _, m := range d.Marriages {
		if m.DivorceDate != "" {
			continue
		}
		s := "Verheiratet mit: " + m.Spouse.Name
		if m.WeddingDate != "" {
			s += " seit " + date(m.WeddingDate)
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " · ")
}
