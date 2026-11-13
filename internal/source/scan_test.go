package source

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/t-kitabayashi-churadata/okf-lens/internal/okf"
)

func concept(typ string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte("---\ntype: " + typ + "\n---\n本文\n")}
}

func paths(docs []*okf.Document) []string {
	out := make([]string, len(docs))
	for i, d := range docs {
		out[i] = d.Path
	}
	return out
}

func TestScan_collectsConcepts(t *testing.T) {
	fsys := fstest.MapFS{
		"b.md":          concept("API"),
		"a.md":          concept("画面"),
		"dir/c.md":      concept("用語"),
		"dir/sub/d.MD":  concept("課題"),
		"dir/memo.txt":  {Data: []byte("無関係")},
		"dir/image.png": {Data: []byte{0x89, 0x50}},
	}
	res, err := Scan(fsys, Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.md", "b.md", "dir/c.md", "dir/sub/d.MD"}
	if got := paths(res.Docs); !reflect.DeepEqual(got, want) {
		t.Errorf("docs = %v, want %v", got, want)
	}
	if len(res.Skipped) != 0 {
		t.Errorf("skipped = %v", res.Skipped)
	}
}

func TestScan_excludesReservedFiles(t *testing.T) {
	fsys := fstest.MapFS{
		"index.md":       {Data: []byte("---\nokf_version: \"0.2\"\n---\n")},
		"log.md":         concept("X"),
		"dir/index.md":   concept("X"),
		"dir/LOG.md":     concept("X"),
		"dir/concept.md": concept("X"),
		"index.md.bak":   concept("X"),
	}
	res, err := Scan(fsys, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := paths(res.Docs), []string{"dir/concept.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("docs = %v, want %v", got, want)
	}
	// 予約ファイルは「読み飛ばし」ではなく、対象外として記録しない。
	if len(res.Skipped) != 0 {
		t.Errorf("skipped = %v", res.Skipped)
	}
}

func TestScan_skipsHiddenDirs(t *testing.T) {
	fsys := fstest.MapFS{
		"a.md":                concept("X"),
		".git/x.md":           concept("X"),
		".obsidian/note.md":   concept("X"),
		"dir/.hidden/note.md": concept("X"),
		"dir/visible.md":      concept("X"),
	}
	res, err := Scan(fsys, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := paths(res.Docs), []string{"a.md", "dir/visible.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("docs = %v, want %v", got, want)
	}
}

func TestScan_recordsSkipped(t *testing.T) {
	fsys := fstest.MapFS{
		"ok.md":      concept("X"),
		"plain.md":   {Data: []byte("# frontmatter なし\n")},
		"notype.md":  {Data: []byte("---\ntitle: x\n---\n")},
		"open.md":    {Data: []byte("---\ntype: X\n")},
		"badyaml.md": {Data: []byte("---\ntype: [\n---\n")},
	}
	res, err := Scan(fsys, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := paths(res.Docs), []string{"ok.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("docs = %v, want %v", got, want)
	}

	byPath := map[string]error{}
	for _, s := range res.Skipped {
		byPath[s.Path] = s.Err
	}
	checks := map[string]error{
		"plain.md":  okf.ErrNoFrontmatter,
		"notype.md": okf.ErrMissingType,
		"open.md":   okf.ErrUnterminatedFrontmatter,
	}
	for p, want := range checks {
		if !errors.Is(byPath[p], want) {
			t.Errorf("%s: err = %v, want %v", p, byPath[p], want)
		}
	}
	if byPath["badyaml.md"] == nil {
		t.Error("badyaml.md が記録されていない")
	}
	if len(res.Skipped) != 4 {
		t.Errorf("skipped = %d 件, want 4 件", len(res.Skipped))
	}
}

func TestScan_empty(t *testing.T) {
	res, err := Scan(fstest.MapFS{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Docs) != 0 || len(res.Skipped) != 0 {
		t.Errorf("res = %+v", res)
	}
}

func TestScanDir(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.md", "---\r\ntype: API\r\n---\r\n本文\r\n")
	write("設計/認証.md", "---\ntype: 画面\n---\n")
	write("index.md", "---\nokf_version: \"0.2\"\n---\n")

	res, err := ScanDir(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// パス区切りは OS によらず "/" になる。
	if got, want := paths(res.Docs), []string{"a.md", "設計/認証.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("docs = %v, want %v", got, want)
	}
}

func TestScanDir_errors(t *testing.T) {
	if _, err := ScanDir(filepath.Join(t.TempDir(), "missing"), Options{}); err == nil {
		t.Error("存在しないディレクトリはエラーになるはず")
	}
	file := filepath.Join(t.TempDir(), "f.md")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ScanDir(file, Options{}); err == nil {
		t.Error("ファイルを渡すとエラーになるはず")
	}
}
