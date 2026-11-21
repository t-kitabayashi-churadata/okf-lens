package stamp

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestScan_states(t *testing.T) {
	root := tree(t, map[string]string{
		"ok.md":     "---\ntype: API\ntitle: x\n---\n# 見出し\n",
		"notype.md": "---\ntitle: x\n---\n本文\n",
		"none.md":   "# 見出しだけ\n```\n# コード内\n```\n",
		"broken.md": "---\ntype: [\n---\n",
		"open.md":   "---\ntype: API\n",
	})
	files := []string{"broken.md", "none.md", "notype.md", "ok.md", "open.md"}
	got := Scan(root, files)
	want := []ScanEntry{
		{Path: "broken.md", State: StateBroken, Keys: []string{}},
		{Path: "none.md", State: StateNoFrontmatter, Keys: []string{}, H1: "見出しだけ"},
		{Path: "notype.md", State: StateNoType, Keys: []string{"title"}},
		{Path: "ok.md", State: StateOK, Keys: []string{"type", "title"}, H1: "見出し"},
		{Path: "open.md", State: StateBroken, Keys: []string{}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestResolveTargets(t *testing.T) {
	root := tree(t, map[string]string{
		"a/x.md": "x", "a/index.md": "x", "a/sub/y.md": "y", "b.md": "b", "a/n.txt": "n",
	})
	tg, err := ResolveTargets(root, []string{"a"}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tg.Files, []string{"a/sub/y.md", "a/x.md"}) {
		t.Errorf("files = %v", tg.Files)
	}
	tg, _ = ResolveTargets(root, []string{"a"}, true, nil)
	if !reflect.DeepEqual(tg.Files, []string{"a/x.md"}) {
		t.Errorf("no-recurse files = %v", tg.Files)
	}
	tg, _ = ResolveTargets(root, []string{"a/index.md"}, false, nil)
	if len(tg.Files) != 0 || len(tg.Skipped) != 1 || tg.Skipped[0].Error.Code != CodeReservedName {
		t.Errorf("reserved = %+v", tg)
	}
	if _, err := ResolveTargets(root, nil, false, nil); err != ErrNoTargets {
		t.Errorf("err = %v", err)
	}
	if _, err := ResolveTargets(root, []string{"../x"}, false, nil); err == nil {
		t.Error("境界の外は PathError")
	}
}

func files(prefix string, n int) []string {
	var out []string
	for i := 0; i < n; i++ {
		out = append(out, fmt.Sprintf("%s/f%03d.md", prefix, i))
	}
	return out
}

func TestPartition(t *testing.T) {
	var fl []string
	fl = append(fl, files("api", 5)...)
	fl = append(fl, files("screen", 3)...)
	fl = append(fl, files("screen/pc", 4)...)
	fl = append(fl, files("docs/spec", 7)...)
	got := Partition(fl, []string{"."}, 6)
	if got.Total != 19 {
		t.Errorf("total = %d", got.Total)
	}
	sum := 0
	for _, g := range got.Groups {
		sum += g.Files
	}
	if sum != got.Total {
		t.Errorf("グループの合計 %d が総数 %d と違う: %+v", sum, got.Total, got.Groups)
	}
	b, _ := json.Marshal(got.Groups)
	s := string(b)
	for _, w := range []string{
		`{"path":"api","recursive":true,"files":5}`,
		`{"path":"docs/spec","recursive":false,"files":7,"oversize":true}`,
		`{"path":"screen","recursive":false,"files":3}`,
		`{"path":"screen/pc","recursive":true,"files":4}`,
	} {
		if !strings.Contains(s, w) {
			t.Errorf("%s が含まれない: %s", w, s)
		}
	}
}

func TestPartition_deterministicAndSmall(t *testing.T) {
	fl := append(files("b", 2), files("a", 2)...)
	r1 := Partition(fl, []string{"."}, 100)
	if len(r1.Groups) != 1 || r1.Groups[0].Path != "." || !r1.Groups[0].Recursive || r1.Groups[0].Files != 4 {
		t.Errorf("groups = %+v", r1.Groups)
	}
	r2 := Partition(fl, []string{"."}, 3)
	if r2.Groups[0].Path != "a" || r2.Groups[1].Path != "b" {
		t.Errorf("辞書順になっていない: %+v", r2.Groups)
	}
}

func TestSchemaIsValidJSON(t *testing.T) {
	var v map[string]any
	if err := json.Unmarshal([]byte(Schema), &v); err != nil {
		t.Fatal(err)
	}
	if v["additionalProperties"] != false {
		t.Error("未知のトップレベルキーを許している(設計書 6 章はエラー)")
	}
}

func TestPartition_filePaths(t *testing.T) {
	fl := []string{"a/x.md", "a/y.md", "b.md"}
	// a はフォルダの PATH、b.md と a/x.md はファイルの PATH。a/x.md は a の配下なので重複させない。
	got := Partition(fl, []string{"a"}, 100)
	if len(got.Groups) != 2 || got.Groups[0].Path != "a" || got.Groups[0].Files != 2 ||
		got.Groups[1].Path != "b.md" || got.Groups[1].Recursive || got.Groups[1].Files != 1 {
		t.Errorf("groups = %+v", got.Groups)
	}
	if got.Total != 3 {
		t.Errorf("total = %d", got.Total)
	}
}
