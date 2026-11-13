package query

import (
	"reflect"
	"sort"
	"testing"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/t-kitabayashi-churadata/okf-lens/internal/okf"
)

func doc(path, typ, title string, mod func(*okf.Document)) *okf.Document {
	d := &okf.Document{Path: path, Type: typ, Title: title, Fields: map[string]any{}}
	if mod != nil {
		mod(d)
	}
	return d
}

// paths は結果のパスを返す。0 件のときは nil を返す。
func paths(ms []Match) []string {
	var out []string
	for _, m := range ms {
		out = append(out, m.Doc.Path)
	}
	return out
}

func TestSearch_noConditionReturnsAllInPathOrder(t *testing.T) {
	docs := []*okf.Document{doc("b.md", "X", "b", nil), doc("a.md", "X", "a", nil)}
	got := paths(Search(docs, Query{}))
	if want := []string{"a.md", "b.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSearch_text(t *testing.T) {
	docs := []*okf.Document{
		doc("desc.md", "X", "無関係", func(d *okf.Document) { d.Description = "認証の説明" }),
		doc("alias.md", "X", "無関係", func(d *okf.Document) { d.Aliases = []string{"認証ガイド"} }),
		doc("partial.md", "X", "認証設計", nil),
		doc("exact.md", "X", "認証", nil),
		doc("none.md", "X", "決済", nil),
	}
	got := paths(Search(docs, Query{Text: "認証"}))
	want := []string{"exact.md", "partial.md", "alias.md", "desc.md"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSearch_textIgnoresCase(t *testing.T) {
	docs := []*okf.Document{doc("a.md", "X", "Auth API", nil)}
	if got := Search(docs, Query{Text: "aUTH"}); len(got) != 1 {
		t.Errorf("got %d 件", len(got))
	}
}

func TestSearch_textMultipleTermsAreAND(t *testing.T) {
	docs := []*okf.Document{
		doc("both.md", "X", "認証API", nil),
		doc("one.md", "X", "認証", nil),
		doc("split.md", "X", "認証", func(d *okf.Document) { d.Description = "APIの説明" }),
	}
	got := paths(Search(docs, Query{Text: "認証  API"}))
	want := []string{"both.md", "split.md"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSearch_textNFC(t *testing.T) {
	docs := []*okf.Document{doc("a.md", "X", "ガイド", nil)}
	got := Search(docs, Query{Text: norm.NFD.String("ガイド")})
	if len(got) != 1 {
		t.Errorf("NFD のクエリが一致しない")
	}
}

func TestSearch_scoresTieBrokenByPath(t *testing.T) {
	docs := []*okf.Document{doc("b.md", "X", "認証B", nil), doc("a.md", "X", "認証A", nil)}
	got := paths(Search(docs, Query{Text: "認証"}))
	if want := []string{"a.md", "b.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSearch_tags(t *testing.T) {
	withTags := func(tags ...string) func(*okf.Document) {
		return func(d *okf.Document) { d.Tags = tags }
	}
	docs := []*okf.Document{
		doc("a.md", "X", "a", withTags("設計/基本設計", "api")),
		doc("b.md", "X", "b", withTags("設計")),
		doc("c.md", "X", "c", withTags("設計書")),
		doc("d.md", "X", "d", withTags("API")),
		doc("e.md", "X", "e", nil),
	}
	tests := []struct {
		name string
		tags []string
		want []string
	}{
		{"階層一致", []string{"設計"}, []string{"a.md", "b.md"}},
		{"下位の階層", []string{"設計/基本設計"}, []string{"a.md"}},
		{"上位だけの文書は下位に一致しない", []string{"設計/詳細設計"}, nil},
		{"前方一致では一致しない", []string{"設"}, nil},
		{"大文字小文字を区別しない", []string{"api"}, []string{"a.md", "d.md"}},
		{"複数指定は AND", []string{"設計", "api"}, []string{"a.md"}},
		{"# 付きも受け付ける", []string{"#api"}, []string{"a.md", "d.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paths(Search(docs, Query{Tags: tt.tags}))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSearch_types(t *testing.T) {
	docs := []*okf.Document{
		doc("a.md", "API", "a", nil),
		doc("b.md", "画面", "b", nil),
		doc("c.md", "用語", "c", nil),
	}
	tests := []struct {
		name  string
		types []string
		want  []string
	}{
		{"単一", []string{"API"}, []string{"a.md"}},
		{"大文字小文字を区別しない", []string{"api"}, []string{"a.md"}},
		{"複数指定は OR", []string{"API", "画面"}, []string{"a.md", "b.md"}},
		{"部分一致はしない", []string{"AP"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paths(Search(docs, Query{Types: tt.types}))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSearch_where(t *testing.T) {
	fields := func(f map[string]any) func(*okf.Document) {
		return func(d *okf.Document) { d.Fields = f }
	}
	docs := []*okf.Document{
		doc("draft.md", "X", "a", fields(map[string]any{"status": "draft", "priority": 3, "reviewed": true})),
		doc("stable.md", "X", "b", fields(map[string]any{"status": "stable"})),
		doc("omit.md", "X", "c", fields(map[string]any{})),
		doc("list.md", "X", "d", fields(map[string]any{"owner": []any{"太郎", "花子"}})),
		doc("date.md", "X", "e", fields(map[string]any{"date": time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)})),
		doc("nested.md", "X", "f", fields(map[string]any{"meta": map[string]any{"owner": "太郎"}})),
	}
	tests := []struct {
		name  string
		where map[string]string
		want  []string
	}{
		{"文字列", map[string]string{"status": "draft"}, []string{"draft.md"}},
		{"値の大文字小文字を区別しない", map[string]string{"status": "DRAFT"}, []string{"draft.md"}},
		{"キーの大文字小文字を区別しない", map[string]string{"Status": "draft"}, []string{"draft.md"}},
		{"status 省略は stable", map[string]string{"status": "stable"}, []string{"list.md", "date.md", "nested.md", "omit.md", "stable.md"}},
		{"数値", map[string]string{"priority": "3"}, []string{"draft.md"}},
		{"真偽値", map[string]string{"reviewed": "true"}, []string{"draft.md"}},
		{"リストは要素のいずれか", map[string]string{"owner": "花子"}, []string{"list.md"}},
		{"日付", map[string]string{"date": "2026-11-13"}, []string{"date.md"}},
		{"ネストは対象外", map[string]string{"meta": "太郎"}, nil},
		{"存在しないキー", map[string]string{"nothing": "x"}, nil},
		{"複数指定は AND", map[string]string{"status": "draft", "priority": "3"}, []string{"draft.md"}},
		{"AND で不一致", map[string]string{"status": "draft", "priority": "4"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paths(Search(docs, Query{Where: tt.where}))
			// 期待値はパス順に揃える。
			if !reflect.DeepEqual(got, sorted(tt.want)) {
				t.Errorf("got %v, want %v", got, sorted(tt.want))
			}
		})
	}
}

func sorted(in []string) []string {
	if in == nil {
		return nil
	}
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func TestSearch_conditionsAreAND(t *testing.T) {
	docs := []*okf.Document{
		doc("hit.md", "API", "認証", func(d *okf.Document) { d.Tags = []string{"設計/基本設計"} }),
		doc("wrongtype.md", "画面", "認証", func(d *okf.Document) { d.Tags = []string{"設計"} }),
		doc("wrongtag.md", "API", "認証", nil),
		doc("wrongtext.md", "API", "決済", func(d *okf.Document) { d.Tags = []string{"設計"} }),
	}
	got := paths(Search(docs, Query{Text: "認証", Tags: []string{"設計"}, Types: []string{"API"}}))
	if want := []string{"hit.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
