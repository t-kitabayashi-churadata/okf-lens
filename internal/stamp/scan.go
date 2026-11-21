package stamp

import (
	"errors"
	"os"
	"path/filepath"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/t-kitabayashi-churadata/okf-lens/internal/okf"
)

// ファイルの状態(設計書 7 章)。
const (
	StateOK            = "ok"             // frontmatter があり、type がある
	StateNoType        = "no-type"        // frontmatter はあるが type がない
	StateNoFrontmatter = "no-frontmatter" // frontmatter がない
	StateBroken        = "broken"         // YAML が不正、終端がない、読めない。apply は触らない
)

// ScanEntry は scan の1ファイルの結果。
type ScanEntry struct {
	Path  string   `json:"path"`
	State string   `json:"state"`
	Keys  []string `json:"keys"`
	H1    string   `json:"h1"`
}

// Scan は targets の各ファイルの状態を調べる。ファイルは変更しない。
func Scan(root string, files []string) []ScanEntry {
	out := make([]ScanEntry, 0, len(files))
	for _, f := range files {
		out = append(out, scanFile(root, f))
	}
	return out
}

func scanFile(root, rel string) ScanEntry {
	e := ScanEntry{Path: rel, State: StateBroken, Keys: []string{}}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil || !utf8.Valid(data) {
		return e
	}
	parts, err := okf.SplitRaw(data)
	switch {
	case errors.Is(err, okf.ErrNoFrontmatter):
		e.State = StateNoFrontmatter
		e.H1 = okf.FirstH1(string(data[bomLen(parts):]))
		return e
	case err != nil:
		return e
	}
	pairs, perr := rootPairs(data[parts.FrontStart:parts.FrontEnd])
	if perr != nil {
		return e
	}
	e.State = StateNoType
	for _, p := range pairs {
		e.Keys = append(e.Keys, p.key.Value)
		if p.key.Value == "type" && p.val.Kind == yaml.ScalarNode && p.val.Value != "" {
			e.State = StateOK
		}
	}
	e.H1 = okf.FirstH1(string(data[parts.BodyStart:]))
	return e
}

func bomLen(p okf.RawParts) int {
	if p.BOM {
		return len("\xEF\xBB\xBF")
	}
	return 0
}
