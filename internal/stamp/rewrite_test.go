package stamp

import (
	"strings"
	"testing"

	"github.com/t-kitabayashi-churadata/okf-lens/internal/okf"
)

func valid(t *testing.T, in map[string]any) map[string]any {
	t.Helper()
	out, errs := Validate(in)
	if errs != nil {
		t.Fatalf("Validate: %v", errs)
	}
	return out
}

func apply(t *testing.T, data string, in map[string]any, mode Mode) (*Change, []*Error) {
	t.Helper()
	return Apply("a.md", []byte(data), valid(t, in), mode)
}

func mustApply(t *testing.T, data string, in map[string]any, mode Mode) *Change {
	t.Helper()
	ch, errs := apply(t, data, in, mode)
	if errs != nil {
		t.Fatalf("Apply: %v", errs)
	}
	return ch
}

func TestApply_newFrontmatter(t *testing.T) {
	ch := mustApply(t, "# 認証\n本文\n", map[string]any{
		"type": "API", "title": "認証API", "tags": []any{"設計/基本設計", "#api"},
		"owner": "team-a",
	}, ModeFill)
	want := "---\ntype: API\ntitle: 認証API\ntags:\n  - 設計/基本設計\n  - api\nowner: team-a\n---\n\n# 認証\n本文\n"
	if string(ch.New) != want {
		t.Errorf("got:\n%q\nwant:\n%q", ch.New, want)
	}
	if ch.Action != ActionAdd {
		t.Errorf("action = %s", ch.Action)
	}
}

func TestApply_newFrontmatter_CRLFandBOM(t *testing.T) {
	ch := mustApply(t, "\xEF\xBB\xBF# 認証\r\n本文\r\n", map[string]any{"type": "API"}, ModeFill)
	want := "\xEF\xBB\xBF---\r\ntype: API\r\n---\r\n\r\n# 認証\r\n本文\r\n"
	if string(ch.New) != want {
		t.Errorf("got %q, want %q", ch.New, want)
	}
}

func TestApply_newFrontmatter_emptyBody(t *testing.T) {
	ch := mustApply(t, "", map[string]any{"type": "API"}, ModeFill)
	if string(ch.New) != "---\ntype: API\n---\n" {
		t.Errorf("got %q", ch.New)
	}
}

func TestApply_addKeepsExistingBytes(t *testing.T) {
	src := "---\n# 認証の概要\ntype:   'API'   # 種別\nowner: team-a\n---\n本文\n"
	ch := mustApply(t, src, map[string]any{"type": "画面", "title": "認証API"}, ModeFill)
	want := "---\n# 認証の概要\ntype:   'API'   # 種別\nowner: team-a\ntitle: 認証API\n---\n本文\n"
	if string(ch.New) != want {
		t.Errorf("got:\n%q\nwant:\n%q", ch.New, want)
	}
	if strings.Join(ch.Kept, ",") != "type" || strings.Join(ch.Added, ",") != "title" {
		t.Errorf("kept=%v added=%v", ch.Kept, ch.Added)
	}
}

func TestApply_addToCRLF(t *testing.T) {
	src := "---\r\ntype: API\r\n---\r\n本文\r\n"
	ch := mustApply(t, src, map[string]any{"tags": []any{"a"}}, ModeFill)
	want := "---\r\ntype: API\r\ntags:\r\n  - a\r\n---\r\n本文\r\n"
	if string(ch.New) != want {
		t.Errorf("got %q, want %q", ch.New, want)
	}
}

func TestApply_emptyFrontmatter(t *testing.T) {
	ch := mustApply(t, "---\n---\n本文\n", map[string]any{"type": "API"}, ModeFill)
	if string(ch.New) != "---\ntype: API\n---\n本文\n" {
		t.Errorf("got %q", ch.New)
	}
}

func TestApply_overwriteReplacesOnlyKeyRange(t *testing.T) {
	src := "---\n" +
		"type: API\n" +
		"# タグの説明\n" +
		"tags:\n" +
		"  - old1\n" +
		"  - old2\n" +
		"\n" +
		"# 担当\n" +
		"owner: team-a\n" +
		"---\n本文\n"
	ch := mustApply(t, src, map[string]any{"tags": []any{"new"}, "type": "画面"}, ModeOverwrite)
	want := "---\n" +
		"type: 画面\n" +
		"# タグの説明\n" +
		"tags:\n" +
		"  - new\n" +
		"\n" +
		"# 担当\n" +
		"owner: team-a\n" +
		"---\n本文\n"
	if string(ch.New) != want {
		t.Errorf("got:\n%s\nwant:\n%s", ch.New, want)
	}
	if strings.Join(ch.Overwritten, ",") != "type,tags" {
		t.Errorf("overwritten = %v", ch.Overwritten)
	}
}

func TestApply_overwriteLastKey(t *testing.T) {
	src := "---\ntype: API\ntags: [a, b]\n---\n"
	ch := mustApply(t, src, map[string]any{"tags": []any{"c"}}, ModeOverwrite)
	if string(ch.New) != "---\ntype: API\ntags:\n  - c\n---\n" {
		t.Errorf("got %q", ch.New)
	}
}

func TestApply_merge(t *testing.T) {
	src := "---\ntype: API\ntags: [a, B]\ntitle: 既存\n---\n"
	ch := mustApply(t, src, map[string]any{
		"tags": []any{"b", "c"}, "title": "新", "aliases": []any{"x"},
	}, ModeMerge)
	want := "---\ntype: API\ntags:\n  - a\n  - B\n  - c\ntitle: 既存\naliases:\n  - x\n---\n"
	if string(ch.New) != want {
		t.Errorf("got:\n%s\nwant:\n%s", ch.New, want)
	}
	if strings.Join(ch.Kept, ",") != "title" {
		t.Errorf("kept = %v", ch.Kept)
	}
}

func TestApply_mergeCommaSeparatedExisting(t *testing.T) {
	src := "---\ntype: API\ntags: a, b\n---\n"
	ch := mustApply(t, src, map[string]any{"tags": []any{"c"}}, ModeMerge)
	if string(ch.New) != "---\ntype: API\ntags:\n  - a\n  - b\n  - c\n---\n" {
		t.Errorf("got %q", ch.New)
	}
}

func TestApply_idempotent(t *testing.T) {
	in := map[string]any{
		"type": "API", "title": "認証API", "tags": []any{"a", "b"},
		"stale_after": "2026-12-31T00:00:00Z", "priority": int64(3),
	}
	for _, mode := range []Mode{ModeFill, ModeOverwrite, ModeMerge} {
		first := mustApply(t, "本文\n", in, mode)
		second := mustApply(t, string(first.New), in, mode)
		if second.Action != ActionUnchanged || second.New != nil {
			t.Errorf("%s: 2回目は unchanged のはず: %+v", mode, second)
		}
	}
}

func TestApply_roundTripWithParse(t *testing.T) {
	ch := mustApply(t, "本文\n", map[string]any{
		"type": "API", "title": "認証API", "description": strings.Repeat("長い説明 ", 40),
		"tags": []any{"設計/基本設計"}, "aliases": []any{"Auth: API", "true", "2026-11-20"},
		"nested": map[string]any{"a": []any{int64(1), "x: y"}},
	}, ModeFill)
	doc, err := okf.Parse("a.md", ch.New)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Type != "API" || doc.Title != "認証API" {
		t.Errorf("type/title = %q/%q", doc.Type, doc.Title)
	}
	if got := strings.Join(doc.Aliases, "|"); got != "Auth: API|true|2026-11-20" {
		t.Errorf("aliases = %q", got)
	}
	if strings.Count(string(ch.New), "\n") > 20 {
		t.Errorf("長い文字列が折り返されている:\n%s", ch.New)
	}
	if len(doc.Warnings) != 0 {
		t.Errorf("warnings = %v", doc.Warnings)
	}
}

func TestApply_errors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		in   map[string]any
		code string
	}{
		{"type がない(frontmatter なし)", "本文\n", map[string]any{"title": "x"}, CodeMissingType},
		{"type がない(既存にもない)", "---\ntitle: x\n---\n", map[string]any{"tags": []any{"a"}}, CodeMissingType},
		{"終端なし", "---\ntype: API\n本文\n", map[string]any{"type": "API"}, CodeBrokenFrontmatter},
		{"YAML 不正", "---\ntype: [\n---\n", map[string]any{"title": "x"}, CodeBrokenFrontmatter},
		{"マップでない", "---\n- a\n- b\n---\n", map[string]any{"type": "API"}, CodeBrokenFrontmatter},
		{"UTF-8 でない", "---\ntype: \xff\n---\n", map[string]any{"title": "x"}, CodeNotUTF8},
		{"CR だけの改行", "---\rtype: API\r---\r", map[string]any{"title": "x"}, CodeBrokenFrontmatter},
		{"既存の type が文字列でない", "---\ntype: 123\n---\n", map[string]any{"title": "x"}, CodeInvalidValue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch, errs := apply(t, tt.src, tt.in, ModeFill)
			if len(errs) != 1 || errs[0].Code != tt.code {
				t.Fatalf("errs = %v, want code %s", errs, tt.code)
			}
			if ch != nil && ch.New != nil {
				t.Errorf("エラーなのに内容を返している: %q", ch.New)
			}
		})
	}
}

func TestApply_typeFilledIntoExistingFrontmatter(t *testing.T) {
	ch := mustApply(t, "---\ntitle: x\n---\n本文\n", map[string]any{"type": "API"}, ModeFill)
	if string(ch.New) != "---\ntitle: x\ntype: API\n---\n本文\n" {
		t.Errorf("got %q", ch.New)
	}
}

// overwrite で置き換えた tags と aliases も、書き込み前の検証の対象になる。
func TestFinalCheck_verifiesOverwrittenLists(t *testing.T) {
	content := []byte("---\ntype: API\ntags:\n  - actual\naliases:\n  - actual\n---\n")
	for _, key := range []string{"tags", "aliases"} {
		ch := &Change{Action: ActionUpdate, Overwritten: []string{key}}
		input := map[string]any{"type": "API", key: []string{"expected"}}
		errs := finalCheck("a.md", content, input, ch, ModeOverwrite, false)
		if len(errs) != 1 || errs[0].Code != CodeVerifyFailed {
			t.Errorf("%s: errs = %v, want verify_failed", key, errs)
		}
		// merge では既存の要素を含むため、同じ内容でも検証しない。
		if errs := finalCheck("a.md", content, input, ch, ModeMerge, false); errs != nil {
			t.Errorf("%s: merge では検証しない: %v", key, errs)
		}
	}
}

// stale_after は引用符なしの日時として書く。既存の同じ値は書き直さない。
func TestApply_staleAfterIsTimestamp(t *testing.T) {
	in := map[string]any{"type": "API", "stale_after": "2026-12-31T00:00:00+09:00"}
	ch := mustApply(t, "本文\n", in, ModeOverwrite)
	want := "---\ntype: API\nstale_after: 2026-12-31T00:00:00+09:00\n---\n\n本文\n"
	if string(ch.New) != want {
		t.Errorf("got %q, want %q", ch.New, want)
	}
	// 既存が引用符なしの同じ日時なら、overwrite でも unchanged。
	src := "---\ntype: API\nstale_after: 2026-12-31T00:00:00Z\n---\n"
	same := map[string]any{"type": "API", "stale_after": "2026-12-31T00:00:00Z"}
	if ch := mustApply(t, src, same, ModeOverwrite); ch.Action != ActionUnchanged {
		t.Errorf("引用符なしの同じ値は unchanged のはず: %+v", ch)
	}
	// 既存が引用符つきの文字列なら、日時型に直す。
	quoted := "---\ntype: API\nstale_after: \"2026-12-31T00:00:00Z\"\n---\n"
	ch = mustApply(t, quoted, same, ModeOverwrite)
	if string(ch.New) != src {
		t.Errorf("got %q, want %q", ch.New, src)
	}
}

func TestApply_changesHaveBeforeAndAfter(t *testing.T) {
	data := []byte("---\ntype: Memo\ntitle: 認証\nstatus: draft\n---\n本文\n")
	ch, errs := Apply("a.md", data, map[string]any{"type": "API", "status": "stable", "title": "認証", "resource": "r"}, ModeOverwrite)
	if errs != nil {
		t.Fatal(errs)
	}
	got := map[string]KeyChange{}
	for _, c := range ch.Changes {
		got[c.Key] = c
	}
	if len(got) != 3 || got["type"].Before != "Memo" || got["type"].After != "API" ||
		got["status"].Before != "draft" || got["resource"].Before != nil || got["resource"].After != "r" {
		t.Errorf("changes = %+v", ch.Changes)
	}
	if _, ok := got["title"]; ok {
		t.Error("値が同じ title は changes に出さない")
	}
}
