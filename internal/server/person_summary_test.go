package server

import (
	"strings"
	"testing"

	"bearstack/internal/photos"
)

func TestPersonSummaryLinksEscapeNamesAndOmitDivorcedRelations(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	details := photos.PersonDetails{
		BirthDate: "2000-01-02",
		Parents: photos.PersonParents{
			Mother: &photos.PersonParent{ID: 11, Name: "<script>mother</script>"},
			Father: &photos.PersonParent{ID: 12, Name: "Father & Son"},
		},
		Siblings: []photos.PersonParent{{ID: 13, Name: "Same name"}, {ID: 14, Name: "Same name"}},
		Marriages: []photos.PersonMarriage{
			{Spouse: photos.PersonParent{ID: 15, Name: "Current"}, WeddingDate: "2020-03-04"},
			{Spouse: photos.PersonParent{ID: 16, Name: "Former"}, DivorceDate: "2019-01-01"},
		},
	}
	var body strings.Builder
	if err = templates.ExecuteTemplate(&body, "person_summary", details.SummaryParts()); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`Mutter: <a href="/photos/people/11">&lt;script&gt;mother&lt;/script&gt;</a>`,
		`Vater: <a href="/photos/people/12">Father &amp; Son</a>`,
		`Geschwister: <a href="/photos/people/13">Same name</a>, <a href="/photos/people/14">Same name</a>`,
		`Verheiratet mit: <a href="/photos/people/15">Current</a> seit 04.03.2020`,
	} {
		if !strings.Contains(body.String(), expected) {
			t.Fatalf("missing %s: %s", expected, body.String())
		}
	}
	if strings.Contains(body.String(), "<script>") || strings.Contains(body.String(), "Former") || strings.Contains(body.String(), "/16") {
		t.Fatal(body.String())
	}
}
