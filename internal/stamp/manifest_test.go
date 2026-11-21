package stamp

import (
	"strings"
	"testing"
)

func TestParseManifest_ok(t *testing.T) {
	m, err := ParseManifest(strings.NewReader(`{
	  "version": 1, "mode": "merge",
	  "files": [
	    {"path": "api/auth.md", "mode": "overwrite", "frontmatter": {"type": "API", "priority": 3}},
	    {"path": "screen\\login.md", "frontmatter": {}}
	  ]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.EntryMode(m.Files[0], ModeFill); got != ModeOverwrite {
		t.Errorf("files[0] mode = %s", got)
	}
	if got := m.EntryMode(m.Files[1], ModeFill); got != ModeMerge {
		t.Errorf("files[1] mode = %s", got)
	}
	out, errs := Validate(m.Files[0].Frontmatter)
	if errs != nil || out["priority"] != int64(3) {
		t.Errorf("priority = %#v, errs = %v", out["priority"], errs)
	}
}

func TestParseManifest_errors(t *testing.T) {
	tests := map[string]string{
		"JSON 不正":          `{`,
		"version がない":      `{"files": []}`,
		"version が違う":      `{"version": 2, "files": []}`,
		"未知のトップレベルキー":      `{"version": 1, "files": [], "extra": 1}`,
		"未知の files キー":     `{"version": 1, "files": [{"path": "a.md", "frontmatter": {}, "x": 1}]}`,
		"path がない":         `{"version": 1, "files": [{"frontmatter": {}}]}`,
		"frontmatter がない":  `{"version": 1, "files": [{"path": "a.md"}]}`,
		"mode が不正":         `{"version": 1, "mode": "x", "files": []}`,
		"files[].mode が不正": `{"version": 1, "files": [{"path": "a.md", "mode": "x", "frontmatter": {}}]}`,
		"path が重複":         `{"version": 1, "files": [{"path": "a.md", "frontmatter": {}}, {"path": "./A.md", "frontmatter": {}}]}`,
		"末尾に余分なデータ":        `{"version": 1, "files": []} {}`,
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseManifest(strings.NewReader(in)); err == nil {
				t.Error("エラーになるべき")
			}
		})
	}
}

func TestNormalizePath(t *testing.T) {
	for in, want := range map[string]string{
		`api\auth.md`:   "api/auth.md",
		"./api/auth.md": "api/auth.md",
		"api//auth.md":  "api/auth.md",
		"が.md":         "が.md",
	} {
		if got := NormalizePath(in); got != want {
			t.Errorf("NormalizePath(%q) = %q, want %q", in, got, want)
		}
	}
}
