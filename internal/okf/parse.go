package okf

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
	"gopkg.in/yaml.v3"
)

var (
	// ErrNoFrontmatter は frontmatter がないファイルに対して返る。
	ErrNoFrontmatter = errors.New("okf: frontmatter がない")
	// ErrUnterminatedFrontmatter は frontmatter の終端 "---" がないときに返る。
	ErrUnterminatedFrontmatter = errors.New("okf: frontmatter が閉じていない")
	// ErrMissingType は type がない、または空のときに返る。
	ErrMissingType = errors.New("okf: type がない")
	// ErrInvalidType は type が文字列でないときに返る。
	ErrInvalidType = errors.New("okf: type が文字列でない")
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// Parse は concept ファイルの内容を Document に変換する。
// p は検索ルートからの相対パスで、区切りが "\" でも "/" に変換する。
func Parse(p string, data []byte) (*Document, error) {
	data = bytes.TrimPrefix(data, utf8BOM)
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	front, body, err := splitFrontmatter(text)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := yaml.Unmarshal([]byte(front), &raw); err != nil {
		return nil, fmt.Errorf("okf: frontmatter の YAML が不正: %w", err)
	}

	fields := make(map[string]any, len(raw))
	for k, v := range raw {
		fields[normalize(k)] = normalizeValue(v)
	}

	doc := &Document{
		Path:   strings.ReplaceAll(p, `\`, "/"),
		Fields: fields,
		Body:   norm.NFC.String(body),
	}

	t, ok := fields["type"]
	if !ok || t == nil {
		return nil, ErrMissingType
	}
	ts, ok := t.(string)
	if !ok {
		return nil, ErrInvalidType
	}
	if ts = strings.TrimSpace(ts); ts == "" {
		return nil, ErrMissingType
	}
	doc.Type = ts

	doc.Description = stringField(fields, "description")
	doc.Aliases = stringList(fields["aliases"])
	doc.Tags, doc.Warnings = parseTags(fields["tags"])
	doc.Title = resolveTitle(fields, doc.Body, doc.Path)

	return doc, nil
}

// splitFrontmatter は先頭の "---" で囲まれた YAML と本文に分ける。
func splitFrontmatter(text string) (front, body string, err error) {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], " \t") != "---" {
		return "", "", ErrNoFrontmatter
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], " \t") == "---" {
			front = strings.Join(lines[1:i], "\n")
			body = strings.Join(lines[i+1:], "\n")
			return front, body, nil
		}
	}
	return "", "", ErrUnterminatedFrontmatter
}

func normalize(s string) string { return norm.NFC.String(s) }

// normalizeValue は YAML の値に含まれる文字列を再帰的に NFC へ正規化する。
func normalizeValue(v any) any {
	switch x := v.(type) {
	case string:
		return normalize(x)
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = normalizeValue(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[normalize(k)] = normalizeValue(e)
		}
		return out
	default:
		return v
	}
}

func stringField(fields map[string]any, key string) string {
	s, _ := fields[key].(string)
	return strings.TrimSpace(s)
}

// stringList は文字列、または文字列のリストを []string に変換する。
// 空要素は捨てる。
func stringList(v any) []string {
	var out []string
	add := func(s string) {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	switch x := v.(type) {
	case string:
		add(x)
	case []any:
		for _, e := range x {
			if s, ok := e.(string); ok {
				add(s)
			}
		}
	}
	return out
}

// parseTags は tags をリスト、またはカンマ区切り文字列として読む。
// 不正なタグは読み飛ばし、警告に記録する。
func parseTags(v any) (tags, warnings []string) {
	var candidates []string
	switch x := v.(type) {
	case string:
		candidates = strings.Split(x, ",")
	case []any:
		for _, e := range x {
			switch s := e.(type) {
			case string:
				candidates = append(candidates, s)
			case int, int64, uint64, float64:
				// `tags: [2026]` は数値になる。数字だけのタグとして警告対象にする。
				candidates = append(candidates, fmt.Sprint(s))
			}
		}
	}
	for _, c := range candidates {
		c = strings.TrimPrefix(strings.TrimSpace(c), "#")
		if c == "" {
			continue
		}
		if !validTag(c) {
			warnings = append(warnings, fmt.Sprintf("不正なタグを読み飛ばした: %q", c))
			continue
		}
		tags = append(tags, c)
	}
	return tags, warnings
}

// validTag は Obsidian のタグ規則に従う。
// 使える文字は文字、数字、"_"、"-"、"/"。数字だけのタグと空の階層は不正。
func validTag(tag string) bool {
	for _, seg := range strings.Split(tag, "/") {
		if seg == "" {
			return false
		}
	}
	hasNonDigit := false
	for _, r := range tag {
		switch {
		case unicode.IsLetter(r) || r == '_' || r == '-':
			hasNonDigit = true
		case unicode.IsDigit(r) || r == '/':
		default:
			return false
		}
	}
	return hasNonDigit
}

// resolveTitle は frontmatter の title、本文の最初の H1、ファイル名の順で決める。
func resolveTitle(fields map[string]any, body, p string) string {
	if t := stringField(fields, "title"); t != "" {
		return t
	}
	if h := firstH1(body); h != "" {
		return h
	}
	base := path.Base(p)
	return strings.TrimSuffix(base, path.Ext(base))
}

// firstH1 はコードフェンスの外にある最初の "# 見出し" を返す。
func firstH1(body string) string {
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return ""
}
