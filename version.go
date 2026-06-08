package bearstack

import (
	"embed"
	"strings"
)

//go:embed VERSION
var versionFS embed.FS

func Version() string {
	contents, err := versionFS.ReadFile("VERSION")
	if err != nil {
		return "0.0.0-dev"
	}
	version := strings.TrimSpace(string(contents))
	if version == "" {
		return "0.0.0-dev"
	}
	return version
}
