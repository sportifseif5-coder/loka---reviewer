package rules

import (
	"os"
	"path/filepath"
)

// Load compiles the built-in defaults plus any rule files named in include.
// Include entries are resolved against the repository-local .loka/ dir, the
// repository root, and the user config dir. Missing files produce warnings,
// never errors.
func Load(repoPath string, include []string) ([]*Rule, []string) {
	rs := Defaults()
	var warnings []string

	candidates := func(rel string) []string {
		var out []string
		if repoPath != "" {
			out = append(out, filepath.Join(repoPath, ".loka", rel))
			out = append(out, filepath.Join(repoPath, rel))
		}
		if dir, err := os.UserConfigDir(); err == nil {
			out = append(out, filepath.Join(dir, "loka", rel))
		}
		return out
	}

	for _, rel := range include {
		loaded := false
		for _, p := range candidates(rel) {
			data, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			parsed, err := ParseAll(data, p)
			if err != nil {
				warnings = append(warnings, "rules file "+p+": "+err.Error())
				continue
			}
			rs = append(rs, parsed...)
			loaded = true
			break
		}
		if !loaded {
			warnings = append(warnings, "rules file \""+rel+"\" not found, skipped")
		}
	}
	return rs, warnings
}
