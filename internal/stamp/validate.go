package stamp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/t-kitabayashi-churadata/okf-lens/internal/okf"
)

// 書き込む frontmatter のキーの並び順。ここにないキーは、辞書順で後ろに並べる。
var canonicalOrder = []string{
	"type", "title", "description", "resource",
	"tags", "aliases", "status", "stale_after",
}

var statusValues = map[string]bool{"draft": true, "stable": true, "deprecated": true}

// Validate は書き込む値を正規化して検証する。
//
// 正規化: キーと文字列を NFC にする。JSON の数値を int64 か float64 にする。
// tags の先頭の "#" を除く。tags と aliases の重複を除く。
// 問題があれば、すべて返す。
func Validate(in map[string]any) (map[string]any, []*Error) {
	out := make(map[string]any, len(in))
	var errs []*Error

	for k, v := range in {
		k = norm.NFC.String(k)
		if strings.TrimSpace(k) == "" {
			errs = append(errs, errf(CodeInvalidValue, "空のキーは書けない"))
			continue
		}
		out[k] = convert(v)
	}

	if v, ok := out["type"]; ok {
		s, isStr := v.(string)
		switch {
		case !isStr:
			errs = append(errs, errf(CodeInvalidValue, "type は文字列にする"))
		case strings.TrimSpace(s) == "":
			errs = append(errs, errf(CodeMissingType, "type が空"))
		default:
			out["type"] = strings.TrimSpace(s)
		}
	}

	for _, k := range []string{"title", "description", "resource"} {
		if v, ok := out[k]; ok {
			s, isStr := v.(string)
			if !isStr || strings.TrimSpace(s) == "" {
				errs = append(errs, errf(CodeInvalidValue, k+" は空でない文字列にする"))
				continue
			}
			out[k] = strings.TrimSpace(s)
		}
	}

	if v, ok := out["tags"]; ok {
		list, e := stringList("tags", v)
		errs = append(errs, e...)
		if e == nil {
			var tags []string
			for _, t := range list {
				t = strings.TrimPrefix(t, "#")
				if !okf.ValidTag(t) {
					errs = append(errs, errf(CodeInvalidTag, fmt.Sprintf("不正なタグ: %q", t)))
					continue
				}
				tags = append(tags, t)
			}
			out["tags"] = dedupe(tags)
		}
	}

	if v, ok := out["aliases"]; ok {
		list, e := stringList("aliases", v)
		errs = append(errs, e...)
		if e == nil {
			out["aliases"] = dedupe(list)
		}
	}

	if v, ok := out["status"]; ok {
		if s, isStr := v.(string); !isStr || !statusValues[s] {
			errs = append(errs, errf(CodeInvalidStatus,
				fmt.Sprintf("status は draft、stable、deprecated のいずれか: %v", v)))
		}
	}

	if v, ok := out["stale_after"]; ok {
		s, isStr := v.(string)
		if !isStr {
			errs = append(errs, errf(CodeInvalidValue, "stale_after は文字列にする"))
		} else if t, err := time.Parse(time.RFC3339, s); err != nil {
			errs = append(errs, errf(CodeInvalidValue,
				fmt.Sprintf("stale_after は UTC オフセット付きの ISO 8601(例: 2026-12-31T00:00:00Z): %q", s)))
		} else {
			// 日時型で持つ。YAML には引用符なしで書かれ、既存の引用符なしの値と
			// 同じ表記になる(overwrite の再実行で書き直しが起きない)。
			out["stale_after"] = t
		}
	}

	if len(errs) > 0 {
		return nil, errs
	}
	return out, nil
}

// convert は JSON から読んだ値を、YAML に書ける値へ変換する。
func convert(v any) any {
	switch x := v.(type) {
	case string:
		return norm.NFC.String(x)
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i
		}
		f, _ := x.Float64()
		return f
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = convert(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[norm.NFC.String(k)] = convert(e)
		}
		return out
	default:
		return v
	}
}

// stringList は文字列のリストを []string にする。空要素はエラー。
func stringList(key string, v any) ([]string, []*Error) {
	list, ok := v.([]any)
	if !ok {
		return nil, []*Error{errf(CodeInvalidValue, key+" は文字列のリストにする")}
	}
	var out []string
	var errs []*Error
	for _, e := range list {
		s, isStr := e.(string)
		if !isStr || strings.TrimSpace(s) == "" {
			errs = append(errs, errf(CodeInvalidValue, key+" の要素は空でない文字列にする"))
			continue
		}
		out = append(out, strings.TrimSpace(s))
	}
	if errs != nil {
		return nil, errs
	}
	return out, nil
}

// fold は重複判定用の表記(NFC、小文字)を返す。
func fold(s string) string { return strings.ToLower(norm.NFC.String(s)) }

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if k := fold(s); !seen[k] {
			seen[k] = true
			out = append(out, s)
		}
	}
	return out
}

// legacyKeys は、okf-lens が読まない旧形式のキー名と、正しいキー名。
var legacyKeys = [][2]string{{"tag", "tags"}, {"alias", "aliases"}}

// Warnings は、書き込めるが注意が必要な点を返す(コード legacy_key)。
// 単数形の tag、alias は okf-lens の専用キー(tags、aliases)ではない。
// 任意キーとして --where では検索できるが、--tag や別名検索の対象にならない。
// OKF は未知のキーを拒否しないので、エラーにはせず、そのまま書く。
func Warnings(validated map[string]any) []*Error {
	var out []*Error
	for _, l := range legacyKeys {
		if _, ok := validated[l[0]]; ok {
			out = append(out, errf(CodeLegacyKey, fmt.Sprintf(
				"%s は okf-lens の専用キーではない。--tag、--alias 相当の検索には使われず、--where %s=... でだけ検索できる。%s の誤りなら、%s に直す",
				l[0], l[0], l[1], l[1])))
		}
	}
	return out
}

// orderedKeys は書き込む順にキーを並べる。
func orderedKeys(m map[string]any) []string {
	var keys []string
	used := map[string]bool{}
	for _, k := range canonicalOrder {
		if _, ok := m[k]; ok {
			keys = append(keys, k)
			used[k] = true
		}
	}
	var rest []string
	for k := range m {
		if !used[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	return append(keys, rest...)
}
