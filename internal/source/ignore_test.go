package source

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func TestMatcher(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		isDir   bool
		want    bool
	}{
		// "/" を含まないパターンは、任意の階層の名前に一致する。
		{"名前: ルート", "README.md", "README.md", false, true},
		{"名前: 深い階層", "README.md", "a/b/README.md", false, true},
		{"名前: 別名には一致しない", "README.md", "a/README.md.bak", false, false},
		{"名前: ディレクトリにも一致", "draft", "a/draft", true, true},
		{"名前: ワイルドカード", "*.tmp.md", "a/x.tmp.md", false, true},
		{"名前: ? は1文字", "a?.md", "ab.md", false, true},
		{"名前: ? は0文字に一致しない", "a?.md", "a.md", false, false},
		// 末尾 "/" はディレクトリだけ。
		{"dir のみ: ディレクトリ", "draft/", "a/draft", true, true},
		{"dir のみ: ファイルには一致しない", "draft/", "a/draft", false, false},
		// "/" を含むパターンは、ルートからのパス全体に一致する。
		{"パス: 一致", "docs/draft", "docs/draft", true, true},
		{"パス: 深い階層には一致しない", "docs/draft", "x/docs/draft", true, false},
		{"パス: 先頭の / は無視", "/docs/draft", "docs/draft", true, true},
		{"パス: * は / をまたがない", "docs/*.md", "docs/a/b.md", false, false},
		{"パス: * は同じ階層に一致", "docs/*.md", "docs/b.md", false, true},
		// "**"
		{"**: 0 階層", "docs/**/x.md", "docs/x.md", false, true},
		{"**: 複数階層", "docs/**/x.md", "docs/a/b/x.md", false, true},
		{"**: 先頭", "**/x.md", "a/b/x.md", false, true},
		{"**: 末尾", "docs/**", "docs/a/b.md", false, true},
		{"**: 別のルートには一致しない", "docs/**/x.md", "other/x.md", false, false},
		// 大文字小文字と正規化。
		{"大文字小文字を区別しない", "readme.MD", "A/Readme.md", false, true},
		{"区切りの \\ は / として扱う", `docs\draft`, "docs/draft", true, true},
		{"日本語", "下書き/", "設計/下書き", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, skip, err := parseRule(tt.pattern)
			if err != nil || skip {
				t.Fatalf("parseRule: skip=%v err=%v", skip, err)
			}
			m := &matcher{rules: []rule{r}}
			if got := m.match(tt.path, tt.isDir); got != tt.want {
				t.Errorf("match(%q, dir=%v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
			}
		})
	}
}

func TestParseRule_skipAndErrors(t *testing.T) {
	for _, line := range []string{"", "   ", "# コメント"} {
		if _, skip, err := parseRule(line); !skip || err != nil {
			t.Errorf("%q: skip=%v err=%v", line, skip, err)
		}
	}
	for _, line := range []string{"!keep.md", "/", "a//b", "[abc"} {
		if _, _, err := parseRule(line); !errors.Is(err, ErrInvalidPattern) {
			t.Errorf("%q: err = %v, want ErrInvalidPattern", line, err)
		}
	}
}

func TestNewMatcher_ignoreFileErrorHasLineNumber(t *testing.T) {
	_, err := newMatcher([]byte("ok.md\n# c\n!bad.md\n"), nil)
	if !errors.Is(err, ErrInvalidPattern) {
		t.Fatalf("err = %v", err)
	}
	if want := IgnoreFileName + ":3:"; !strings.Contains(err.Error(), want) {
		t.Errorf("err = %q, 行番号 %q を含むはず", err, want)
	}
}

func TestScan_ignoreFile(t *testing.T) {
	fsys := fstest.MapFS{
		IgnoreFileName:       {Data: []byte("\xEF\xBB\xBF# 除外\r\nREADME.md\r\ndrafts/\r\n\r\ndocs/**/internal.md\r\n")},
		"a.md":               concept("X"),
		"README.md":          {Data: []byte("# frontmatter なし\n")},
		"sub/README.md":      {Data: []byte("# frontmatter なし\n")},
		"drafts/x.md":        concept("X"),
		"sub/drafts/y.md":    concept("X"),
		"docs/a/internal.md": concept("X"),
		"docs/a/open.md":     concept("X"),
	}
	res, err := Scan(fsys, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := paths(res.Docs), []string{"a.md", "docs/a/open.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("docs = %v, want %v", got, want)
	}
	// 除外したファイルは、読み飛ばしにも記録しない。
	if len(res.Skipped) != 0 {
		t.Errorf("skipped = %v", res.Skipped)
	}
}

func TestScan_excludeOption(t *testing.T) {
	fsys := fstest.MapFS{
		IgnoreFileName: {Data: []byte("a.md\n")},
		"a.md":         concept("X"),
		"b.md":         concept("X"),
		"c.md":         concept("X"),
	}
	res, err := Scan(fsys, Options{Exclude: []string{"b.md"}})
	if err != nil {
		t.Fatal(err)
	}
	// ファイルとフラグの両方を適用する。
	if got, want := paths(res.Docs), []string{"c.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("docs = %v, want %v", got, want)
	}
}

func TestScan_invalidPatterns(t *testing.T) {
	_, err := Scan(fstest.MapFS{"a.md": concept("X")}, Options{Exclude: []string{"!a.md"}})
	if !errors.Is(err, ErrInvalidPattern) {
		t.Errorf("--exclude: err = %v", err)
	}
	_, err = Scan(fstest.MapFS{IgnoreFileName: {Data: []byte("[abc\n")}}, Options{})
	if !errors.Is(err, ErrInvalidPattern) {
		t.Errorf("設定ファイル: err = %v", err)
	}
}
