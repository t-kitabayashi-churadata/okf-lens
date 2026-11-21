package stamp

import (
	"reflect"
	"strings"
	"testing"
)

func codes(errs []*Error) string {
	var s []string
	for _, e := range errs {
		s = append(s, e.Code)
	}
	return strings.Join(s, ",")
}

func TestValidate_ok(t *testing.T) {
	out, errs := Validate(map[string]any{
		"type":        " API ",
		"tags":        []any{"#設計/基本設計", "Api", "api"},
		"aliases":     []any{"a", "A"},
		"status":      "draft",
		"stale_after": "2026-12-31T00:00:00+09:00",
		"custom":      map[string]any{"k": "v"},
		"が":          "x", // NFD のキー
	})
	if errs != nil {
		t.Fatal(errs)
	}
	if out["type"] != "API" {
		t.Errorf("type = %v", out["type"])
	}
	if want := []string{"設計/基本設計", "Api"}; !reflect.DeepEqual(out["tags"], want) {
		t.Errorf("tags = %v, want %v", out["tags"], want)
	}
	if want := []string{"a"}; !reflect.DeepEqual(out["aliases"], want) {
		t.Errorf("aliases = %v", out["aliases"])
	}
	if _, ok := out["が"]; !ok {
		t.Errorf("キーが NFC になっていない: %v", out)
	}
}

func TestValidate_errors(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]any
		want string
	}{
		{"type が空", map[string]any{"type": " "}, CodeMissingType},
		{"type が数値", map[string]any{"type": int64(1)}, CodeInvalidValue},
		{"数字だけのタグ", map[string]any{"tags": []any{"2026"}}, CodeInvalidTag},
		{"スペース入りのタグ", map[string]any{"tags": []any{"設計 基本"}}, CodeInvalidTag},
		{"空の階層のタグ", map[string]any{"tags": []any{"設計/"}}, CodeInvalidTag},
		{"tags が文字列", map[string]any{"tags": "a,b"}, CodeInvalidValue},
		{"tags に空要素", map[string]any{"tags": []any{""}}, CodeInvalidValue},
		{"aliases に数値", map[string]any{"aliases": []any{int64(1)}}, CodeInvalidValue},
		{"status が列挙外", map[string]any{"status": "done"}, CodeInvalidStatus},
		{"stale_after にオフセットなし", map[string]any{"stale_after": "2026-12-31"}, CodeInvalidValue},
		{"title が空", map[string]any{"title": ""}, CodeInvalidValue},
		{"複数の問題", map[string]any{"status": "x", "type": ""}, CodeInvalidStatus + "," + CodeMissingType},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, errs := Validate(tt.in)
			got := codes(errs)
			if tt.name == "複数の問題" {
				if !strings.Contains(got, CodeMissingType) || !strings.Contains(got, CodeInvalidStatus) {
					t.Errorf("codes = %s", got)
				}
				return
			}
			if got != tt.want {
				t.Errorf("codes = %q, want %q", got, tt.want)
			}
		})
	}
}

// 単数形の tag、alias はエラーにせず、警告を付けてそのまま書く。
func TestValidate_legacyKeysWarnButPass(t *testing.T) {
	out, errs := Validate(map[string]any{"type": "API", "tag": []any{"a"}, "alias": "x"})
	if errs != nil {
		t.Fatalf("エラーにしない: %v", errs)
	}
	if _, ok := out["tag"]; !ok {
		t.Errorf("tag をそのまま残す: %v", out)
	}
	if got := codes(Warnings(out)); got != CodeLegacyKey+","+CodeLegacyKey {
		t.Errorf("warnings = %q", got)
	}
	if w := Warnings(map[string]any{"tags": []string{"a"}}); w != nil {
		t.Errorf("tags に警告は出さない: %v", w)
	}

	ch := mustApply(t, "本文\n", map[string]any{"type": "API", "tag": []any{"a"}}, ModeFill)
	if want := "---\ntype: API\ntag:\n  - a\n---\n\n本文\n"; string(ch.New) != want {
		t.Errorf("got %q, want %q", ch.New, want)
	}
}
