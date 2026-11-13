package graph

import (
	"reflect"
	"testing"

	"github.com/t-kitabayashi-churadata/okf-lens/internal/okf"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name   string
		from   string
		target string
		want   string
		ok     bool
	}{
		{"絶対パス", "a/b.md", "/tables/customers.md", "tables/customers.md", true},
		{"同じ階層", "a/b.md", "./c.md", "a/c.md", true},
		{"./ なし", "a/b.md", "c.md", "a/c.md", true},
		{"親の階層", "a/b/c.md", "../d.md", "a/d.md", true},
		{"ルート直下から", "a.md", "b.md", "b.md", true},
		{"見出しを除去", "a.md", "b.md#section", "b.md", true},
		{"クエリを除去", "a.md", "b.md?x=1", "b.md", true},
		{"拡張子の省略", "a.md", "/tables/customers", "tables/customers.md", true},
		{"大文字小文字を区別しない", "a.md", "/Tables/Customers.MD", "tables/customers.md", true},
		{"パーセントエンコード", "a.md", "/%E8%AA%8D%E8%A8%BC.md", "認証.md", true},
		{"日本語のパス", "設計/a.md", "./認証.md", "設計/認証.md", true},
		{"空白を含むパスの %20", "a.md", "my%20file.md", "my file.md", true},
		{"NFD は NFC に揃う", "a.md", "/ガ.md", "ガ.md", true},

		{"https", "a.md", "https://example.com/x.md", "", false},
		{"mailto", "a.md", "mailto:a@example.com", "", false},
		{"スキーム相対", "a.md", "//example.com/x.md", "", false},
		{"スコープ記述子", "a.md", "project:all-queries", "", false},
		{"見出しだけ", "a.md", "#section", "", false},
		{"空", "a.md", "  ", "", false},
		{"ディレクトリ", "a.md", "subdir/", "", false},
		{"画像など .md 以外", "a.md", "img.png", "", false},
		{"バンドルの外", "a.md", "../x.md", "", false},
		{"深くバンドルの外", "a/b.md", "../../x.md", "", false},
		{"ルート自身", "a.md", "/", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Resolve(tt.from, tt.target)
			if ok != tt.ok || got != tt.want {
				t.Errorf("Resolve(%q, %q) = (%q, %v), want (%q, %v)", tt.from, tt.target, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestParseTarget(t *testing.T) {
	for in, want := range map[string]string{
		"api/auth.md":    "api/auth.md",
		"api/auth":       "api/auth.md",
		"/api/auth.md":   "api/auth.md",
		"./api/auth.md":  "api/auth.md",
		`api\auth.md`:    "api/auth.md",
		"API/Auth.MD":    "api/auth.md",
		"  api/auth.md ": "api/auth.md",
	} {
		if got := ParseTarget(in); got != want {
			t.Errorf("ParseTarget(%q) = %q, want %q", in, got, want)
		}
	}
}

func mk(path, body string, fields map[string]any) *okf.Document {
	if fields == nil {
		fields = map[string]any{}
	}
	return &okf.Document{Path: path, Type: "X", Body: body, Fields: fields}
}

func TestBuild(t *testing.T) {
	docs := []*okf.Document{
		mk("api/auth.md", "[JWT](/glossary/jwt.md) [自分](./auth.md) [自分2](/api/auth.md) [重複](/glossary/jwt.md#a)", nil),
		mk("screen/login.md", "[認証](../api/auth.md) [JWT](/glossary/jwt.md) [壊れ](/missing.md)", nil),
		mk("glossary/jwt.md", "リンクなし", nil),
		mk("batch/sync.md", "", map[string]any{"computation": "../api/auth.md"}),
	}
	g := Build(docs)

	if got, want := g.Backlinks("glossary/jwt.md"), []string{"api/auth.md", "screen/login.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("jwt の backlinks = %v, want %v", got, want)
	}
	// 本文のリンクと、frontmatter のパス値の両方が数えられる。
	if got, want := g.Backlinks("api/auth.md"), []string{"batch/sync.md", "screen/login.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("auth の backlinks = %v, want %v", got, want)
	}
	// 自己リンクは数えない。重複は1つ。
	if got, want := g.Outlinks("api/auth.md"), []string{"glossary/jwt.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("auth の outlinks = %v, want %v", got, want)
	}
	// 壊れたリンクも保持する。存在しない文書の backlinks を引ける。
	if got, want := g.Backlinks("missing.md"), []string{"screen/login.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("missing の backlinks = %v, want %v", got, want)
	}
	if g.Has("missing.md") || !g.Has("api/auth.md") {
		t.Error("Has が正しくない")
	}
	if p, ok := g.Path("api/auth.md"); !ok || p != "api/auth.md" {
		t.Errorf("Path = %q, %v", p, ok)
	}
	if got := g.Backlinks("batch/sync.md"); len(got) != 0 {
		t.Errorf("リンクされていない文書の backlinks = %v", got)
	}
}

func TestBuild_caseInsensitiveAndOrder(t *testing.T) {
	docs := []*okf.Document{
		mk("B.md", "[x](/Target.md)", nil),
		mk("a.md", "[x](/target.MD)", nil),
		mk("Target.md", "", nil),
	}
	g := Build(docs)
	got := g.Backlinks(Key("target.md"))
	if want := []string{"B.md", "a.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
