package bearstack

import (
	"regexp"
	"testing"
)

func TestVersionIsSemVer(t *testing.T) {
	version := Version()
	semver := regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)
	if !semver.MatchString(version) {
		t.Fatalf("Version() = %q, want semantic version", version)
	}
}
