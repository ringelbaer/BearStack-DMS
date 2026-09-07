package photos

import "testing"

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
