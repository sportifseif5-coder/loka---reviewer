// Package version holds build version information for the loka binaries.
package version

// Version is the semantic version, overridable at build time via -ldflags.
var Version = "0.0.0-dev"

// String renders the full version string.
func String() string {
	return "loka " + Version
}
