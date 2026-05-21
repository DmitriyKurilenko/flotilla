// Package discover finds flotilla project.yml files on disk.
//
// For v0.1 the search glob is hardcoded to /opt/*/project.yml; v0.2
// makes this configurable (see docs/ARCHITECTURE.md §12).
package discover

import (
	"path/filepath"
	"sort"

	"github.com/DmitriyKurilenko/flotilla/internal/project"
)

// DefaultGlob is the convention path used by `--all` in v0.1.
const DefaultGlob = "/opt/*/project.yml"

// Found is one discovered project file.
type Found struct {
	Path string // absolute path to project.yml
	Name string // the parsed `name:` field (used for the alphabetical sort)
	Dir  string // directory containing project.yml (the deploy target)
}

// Discover walks the given globs, parses each project.yml, and returns
// the results sorted alphabetically by Name.
//
// Files that fail to parse or validate are skipped, not fatal — one
// bad project should never block discovery of the others. The error
// return is reserved for failures of the glob itself (a malformed
// pattern), not for individual bad project files.
func Discover(globs []string) ([]Found, error) {
	var found []Found
	seen := make(map[string]struct{})

	for _, g := range globs {
		matches, err := filepath.Glob(g)
		if err != nil {
			return nil, err // only ErrBadPattern per filepath.Glob contract
		}
		for _, m := range matches {
			abs, err := filepath.Abs(m)
			if err != nil {
				continue
			}
			if _, dup := seen[abs]; dup {
				continue
			}
			f, err := project.Load(abs)
			if err != nil {
				continue // skip unparseable/invalid project.yml
			}
			seen[abs] = struct{}{}
			found = append(found, Found{
				Path: abs,
				Name: f.Name,
				Dir:  filepath.Dir(abs),
			})
		}
	}

	sort.Slice(found, func(i, j int) bool {
		if found[i].Name != found[j].Name {
			return found[i].Name < found[j].Name
		}
		return found[i].Path < found[j].Path
	})
	return found, nil
}

// DiscoverDefault is shorthand for Discover([]string{DefaultGlob}).
func DiscoverDefault() ([]Found, error) {
	return Discover([]string{DefaultGlob})
}
