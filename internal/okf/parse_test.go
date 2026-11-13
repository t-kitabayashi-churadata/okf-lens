package okf

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/unicode/norm"
)

func TestParse_basic(t *testing.T) {
	src := "---\ntype: API\ntitle: 認証API\ndescription: ログインとトークン発行\ntags: [基本設計/認証, api]\naliases: [ログインAPI]\n---\n本文\n"
	doc, err := Parse("api/auth.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Type != "API" || doc.Title != "認証API" || doc.Description != "ログインとトークン発行" {
		t.Errorf("unexpected doc: %+v", doc)
	}
	if want := []string{"基本設計/認証", "api"}; !reflect.DeepEqual(doc.Tags, want) {
		t.Errorf("tags = %v, want %v", doc.Tags, want)
	}
	if want := []string{"ログインAPI"}; !reflect.DeepEqual(doc.Aliases, want) {
		t.Errorf("aliases = %v, want %v", doc.Aliases, want)
	}
	if doc.Body != "本文\n" {
		t.Errorf("body = %q", doc.Body)
	}
	if doc.Path != "api/auth.md" {
		t.Errorf("path = %q", doc.Path)
	}
}

func TestParse_CRLFAndBOM(t *testing.T) {
	src := "\xEF\xBB\xBF---\r\ntype: 画面\r\ntags: [a]\r\n---\r\n# 見出し\r\n本文\r\n"
	doc, err := Parse(`screens\login.md`, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Type != "画面" || doc.Title != "見出し" {
		t.Errorf("unexpected doc: %+v", doc)
	}
	if strings.Contains(doc.Body, "\r") {
		t.Errorf("body に CR が残っている: %q", doc.Body)
	}
	if doc.Path != "screens/login.md" {
		t.Errorf("path = %q", doc.Path)
	}
}

func TestParse_NFC(t *testing.T) {
	// macOS が返す NFD 形式("が" = か + 濁点)でも NFC に揃う。
	nfd := norm.NFD.String("ガイド")
	if nfd == "ガイド" {
		t.Fatal("テスト前提が崩れている: NFD と NFC が同じ")
	}
	src := "---\ntype: " + nfd + "\ntitle: " + nfd + "\ntags: [" + nfd + "]\n" + nfd + "キー: " + nfd + "\n---\n"
	doc, err := Parse("a.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Type != "ガイド" || doc.Title != "ガイド" || doc.Tags[0] != "ガイド" {
		t.Errorf("NFC になっていない: %+v", doc)
	}
	if doc.Fields["ガイドキー"] != "ガイド" {
		t.Errorf("Fields が NFC になっていない: %v", doc.Fields)
	}
}

func TestParse_errors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want error
	}{
		{"frontmatter なし", "# だけ\n", ErrNoFrontmatter},
		{"空ファイル", "", ErrNoFrontmatter},
		{"閉じていない", "---\ntype: API\n", ErrUnterminatedFrontmatter},
		{"type なし", "---\ntitle: x\n---\n", ErrMissingType},
		{"type が空", "---\ntype: \"  \"\n---\n", ErrMissingType},
		{"空の frontmatter", "---\n---\n", ErrMissingType},
		{"type が数値", "---\ntype: 1\n---\n", ErrInvalidType},
		{"type がリスト", "---\ntype: [a]\n---\n", ErrInvalidType},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse("a.md", []byte(tt.src))
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestParse_invalidYAML(t *testing.T) {
	_, err := Parse("a.md", []byte("---\ntype: [\n---\n"))
	if err == nil {
		t.Fatal("エラーになるはず")
	}
}

func TestParse_tags(t *testing.T) {
	tests := []struct {
		name     string
		tags     string
		want     []string
		warnings int
	}{
		{"リスト", "[a, b]", []string{"a", "b"}, 0},
		{"カンマ区切り文字列", "\"a, b\"", []string{"a", "b"}, 0},
		{"単一文字列", "a", []string{"a"}, 0},
		{"# 付きは除去", "[\"#a\"]", []string{"a"}, 0},
		{"階層", "[設計/基本設計]", []string{"設計/基本設計"}, 0},
		{"アンダースコアとハイフン", "[a_b, c-d]", []string{"a_b", "c-d"}, 0},
		{"数字だけは不正", "[2026, ok]", []string{"ok"}, 1},
		{"数字を含むのは正常", "[v2, 2026-11]", []string{"v2", "2026-11"}, 0},
		{"スペース含みは不正", "[\"a b\", ok]", []string{"ok"}, 1},
		{"記号は不正", "[\"a!\", ok]", []string{"ok"}, 1},
		{"空の階層は不正", "[\"a//b\", \"a/\", ok]", []string{"ok"}, 2},
		{"空要素は無視", "[\"\", ok]", []string{"ok"}, 0},
		{"tags なし", "", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := "---\ntype: X\n"
			if tt.tags != "" {
				src += "tags: " + tt.tags + "\n"
			}
			src += "---\n"
			doc, err := Parse("a.md", []byte(src))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(doc.Tags, tt.want) {
				t.Errorf("tags = %v, want %v", doc.Tags, tt.want)
			}
			if len(doc.Warnings) != tt.warnings {
				t.Errorf("warnings = %v, want %d 件", doc.Warnings, tt.warnings)
			}
		})
	}
}

func TestParse_aliasesString(t *testing.T) {
	doc, err := Parse("a.md", []byte("---\ntype: X\naliases: 別名\n---\n"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"別名"}; !reflect.DeepEqual(doc.Aliases, want) {
		t.Errorf("aliases = %v", doc.Aliases)
	}
}

func TestParse_titleFallback(t *testing.T) {
	tests := []struct {
		name string
		src  string
		path string
		want string
	}{
		{"frontmatter 優先", "---\ntype: X\ntitle: A\n---\n# B\n", "c.md", "A"},
		{"H1", "---\ntype: X\n---\n# B\n", "c.md", "B"},
		{"コードフェンス内の # は無視", "---\ntype: X\n---\n```sh\n# comment\n```\n# B\n", "c.md", "B"},
		{"H2 は対象外", "---\ntype: X\n---\n## B\n", "dir/c.md", "c"},
		{"ファイル名", "---\ntype: X\n---\n本文\n", "dir/認証設計.md", "認証設計"},
		{"title が空ならフォールバック", "---\ntype: X\ntitle: \"\"\n---\n# B\n", "c.md", "B"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := Parse(tt.path, []byte(tt.src))
			if err != nil {
				t.Fatal(err)
			}
			if doc.Title != tt.want {
				t.Errorf("title = %q, want %q", doc.Title, tt.want)
			}
		})
	}
}

func TestParse_unknownFieldsKept(t *testing.T) {
	src := "---\ntype: X\nstatus: draft\npriority: 3\nreviewed: true\ndate: 2026-11-13\nupdated: 2026-11-13T10:30:00\nmeta:\n  owner: 太郎\nrelated: [\"[[他のノート]]\"]\n---\n"
	doc, err := Parse("a.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Fields["status"] != "draft" {
		t.Errorf("status = %v", doc.Fields["status"])
	}
	if doc.Fields["priority"] != 3 {
		t.Errorf("priority = %v (%T)", doc.Fields["priority"], doc.Fields["priority"])
	}
	if doc.Fields["reviewed"] != true {
		t.Errorf("reviewed = %v", doc.Fields["reviewed"])
	}
	if _, ok := doc.Fields["date"].(time.Time); !ok {
		t.Errorf("date = %v (%T)", doc.Fields["date"], doc.Fields["date"])
	}
	// タイムゾーンなしの日時は yaml.v3 が文字列で返す。解釈は検索側で行う。
	if doc.Fields["updated"] != "2026-11-13T10:30:00" {
		t.Errorf("updated = %v (%T)", doc.Fields["updated"], doc.Fields["updated"])
	}
	meta, ok := doc.Fields["meta"].(map[string]any)
	if !ok || meta["owner"] != "太郎" {
		t.Errorf("meta = %v", doc.Fields["meta"])
	}
	if rel, ok := doc.Fields["related"].([]any); !ok || rel[0] != "[[他のノート]]" {
		t.Errorf("related = %v", doc.Fields["related"])
	}
}

func TestParse_frontmatterDelimiterInBody(t *testing.T) {
	src := "---\ntype: X\n---\n本文\n---\n水平線\n"
	doc, err := Parse("a.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Body != "本文\n---\n水平線\n" {
		t.Errorf("body = %q", doc.Body)
	}
}
