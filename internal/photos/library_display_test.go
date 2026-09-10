package photos

import "testing"

func TestMediaFolderNameUsesGalleryNormalization(t *testing.T) {
	for _, test := range []struct{ path, want string }{
		{"photo.jpg", "Fotos"},
		{"album/photo.jpg", "album"},
		{"2026_07_15_Sommer_Urlaub/photo.jpg", "Sommer Urlaub"},
		{"Archiv/15.03.2024 - Schöne_Ausflüge/photo.jpg", "Schöne Ausflüge"},
		{"2026-05-11/photo.jpg", "2026 05 11"},
		{"Archiv/See & <Berge>/photo.jpg", "See & <Berge>"},
	} {
		t.Run(test.path, func(t *testing.T) {
			if got := MediaFolderName(test.path); got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestMediaDisplayPathUsesGalleryBreadcrumbRules(t *testing.T) {
	for _, test := range []struct{ path, want string }{
		{"IMG_1234.jpg", "Fotos / IMG_1234.jpg"},
		{"2026_05_11_Urlaub/15.03.2024 - Ausflug/IMG_1234.jpg", "Fotos / 11.05.2026 · Urlaub / 15.03.2024 · Ausflug / IMG_1234.jpg"},
		{"2026-05-11/Ältere Bilder/2026_05_11_IMG.jpg", "Fotos / 11.05.2026 · 2026 05 11 / Ältere Bilder / 2026_05_11_IMG.jpg"},
	} {
		if got := mediaDisplayPath(test.path); got != test.want {
			t.Errorf("%q: got %q, want %q", test.path, got, test.want)
		}
	}
}
