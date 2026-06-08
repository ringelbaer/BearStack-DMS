package server

import (
	"strings"
	"testing"
)

func TestDocumentRouteSpecsAreWellFormed(t *testing.T) {
	seen := map[string]struct{}{}
	hasReadOnlyViewRoute := false
	for _, route := range documentRouteSpecs {
		assertWellFormedRouteSpec(t, "documents", route, seen)
		if strings.Contains(route.pattern, "/documents/{id}/view") {
			hasReadOnlyViewRoute = true
		}
	}
	if !hasReadOnlyViewRoute {
		t.Fatal("document route specs missing read-only document view route")
	}
}

func TestApplicationRouteSpecsAreWellFormed(t *testing.T) {
	groups := []struct {
		name   string
		routes []routeSpec
	}{
		{name: "core", routes: coreRouteSpecs},
		{name: "home", routes: homeRouteSpecs(false)},
		{name: "settings", routes: settingsRouteSpecs},
		{name: "photo settings", routes: photoSettingsRouteSpecs},
		{name: "photos", routes: photoRouteSpecs},
		{name: "webdav well-known", routes: webDAVWellKnownRouteSpecs},
		{name: "webdav configured", routes: webDAVRouteSpecs("/dav")},
		{name: "documents", routes: documentRouteSpecs},
	}
	seen := map[string]struct{}{}
	for _, group := range groups {
		if len(group.routes) == 0 {
			t.Fatalf("%s route specs are empty", group.name)
		}
		for _, route := range group.routes {
			assertWellFormedRouteSpec(t, group.name, route, seen)
		}
	}
}

func TestHomeRouteSpecsAllowDocumentsOrPhotosWhenPhotosAreEnabled(t *testing.T) {
	routes := homeRouteSpecs(true)
	if len(routes) != 1 {
		t.Fatalf("home route specs = %d", len(routes))
	}
	route := routes[0]
	if route.pattern != "GET /{$}" {
		t.Fatalf("home route pattern = %q", route.pattern)
	}
	if route.capabilities != authCapDocumentsRead|authCapPhotosRead || !route.requireAny {
		t.Fatalf("home route capabilities = %v requireAny = %t", route.capabilities, route.requireAny)
	}
}

func TestWebDAVRouteSpecsRequireUploadCapabilityOnlyForPut(t *testing.T) {
	for _, route := range webDAVRouteSpecs("/dav") {
		if strings.HasPrefix(route.pattern, "PUT ") {
			if route.capabilities != authCapDocumentsWebDAVRead|authCapDocumentsUpload {
				t.Fatalf("PUT route %q capabilities = %v", route.pattern, route.capabilities)
			}
			continue
		}
		if route.capabilities != authCapDocumentsWebDAVRead {
			t.Fatalf("route %q capabilities = %v", route.pattern, route.capabilities)
		}
	}
}

func TestPhotoRouteSpecsAreSeparatedFromCoreRoutes(t *testing.T) {
	for _, route := range photoRouteSpecs {
		if !strings.Contains(route.pattern, "/photos") {
			t.Fatalf("photo route spec has non-photo pattern %q", route.pattern)
		}
	}
}

func assertWellFormedRouteSpec(t *testing.T, group string, route routeSpec, seen map[string]struct{}) {
	t.Helper()
	if route.pattern == "" {
		t.Fatalf("%s route has empty pattern", group)
	}
	if route.handler == nil {
		t.Fatalf("%s route %q has no handler", group, route.pattern)
	}
	if _, ok := seen[route.pattern]; ok {
		t.Fatalf("duplicate %s route pattern %q", group, route.pattern)
	}
	seen[route.pattern] = struct{}{}
}
