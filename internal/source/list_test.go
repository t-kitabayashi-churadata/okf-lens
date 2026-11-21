package source

import (
	"reflect"
	"testing"
	"testing/fstest"
)

func listFS() fstest.MapFS {
	return fstest.MapFS{
		"a.md":                  concept("API"),
		"memo.md":               {Data: []byte("# frontmatter なし\n")},
		"index.md":              {Data: []byte("---\nokf_version: \"0.2\"\n---\n")},
		"LOG.MD":                {Data: []byte("log")},
		"note.txt":              {Data: []byte("x")},
		"api/auth.md":           concept("API"),
		"api/index.md":          {Data: []byte("目次")},
		"api/v2/token.md":       {Data: []byte("本文のみ")},
		".obsidian/hidden.md":   concept("x"),
		"drafts/wip.md":         concept("x"),
		".okf-lensignore":       {Data: []byte("drafts/\n")},
		"screen/pc/login.md":    concept("画面"),
		"screen/top.md":         concept("画面"),
		"screen/private/pin.md": concept("画面"),
	}
}

func TestList_includesFilesWithoutFrontmatter(t *testing.T) {
	got, err := List(listFS(), ListOptions{Exclude: []string{"private/"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"a.md", "api/auth.md", "api/v2/token.md", "memo.md",
		"screen/pc/login.md", "screen/top.md",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestList_roots(t *testing.T) {
	got, err := List(listFS(), ListOptions{Roots: []string{"api", "api/v2"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"api/auth.md", "api/v2/token.md"} // 重複しない
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestList_noRecurse(t *testing.T) {
	got, err := List(listFS(), ListOptions{Roots: []string{"screen"}, NoRecurse: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"screen/top.md"} // サブフォルダは見ない
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestList_excludedAncestor(t *testing.T) {
	got, err := List(listFS(), ListOptions{Roots: []string{"drafts"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("除外されたディレクトリの配下は返さない: %v", got)
	}
}

func TestList_errors(t *testing.T) {
	for _, root := range []string{"nothing", "a.md", "../x"} {
		if _, err := List(listFS(), ListOptions{Roots: []string{root}}); err == nil {
			t.Errorf("Roots=%q: エラーになるべき", root)
		}
	}
}

func TestIsReserved(t *testing.T) {
	for name, want := range map[string]bool{
		"index.md": true, "INDEX.md": true, "log.md": true, "Log.MD": true,
		"a.md": false, "index.txt": false, "my-index.md": false,
	} {
		if got := IsReserved(name); got != want {
			t.Errorf("IsReserved(%q) = %v, want %v", name, got, want)
		}
	}
}
