package okf

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

var markdown = goldmark.New().Parser()

// ExtractLinks は doc から、リンクの宛先を未解決の文字列のまま返す。
// 重複は除かない。宛先の解決(相対パスの正規化など)は internal/graph が行う。
//
// 対象は OKF v0.2 が定めるリンク:
//   - 本文の Markdown リンク(§6.1)。インライン形式と参照形式を含む。
//     画像、コードブロックとインラインコードの中、URL だけの自動リンクは対象外
//   - frontmatter のパス値フィールド(§6.2): resource、sources[].resource、
//     computation、executor.resource、attester.resource
func ExtractLinks(doc *Document) []string {
	out := bodyLinks(doc.Body)
	return append(out, frontmatterLinks(doc.Fields)...)
}

func bodyLinks(body string) []string {
	root := markdown.Parse(text.NewReader([]byte(body)))
	var out []string
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if l, ok := n.(*ast.Link); ok && entering {
			out = append(out, string(l.Destination))
		}
		return ast.WalkContinue, nil
	})
	return out
}

func frontmatterLinks(f map[string]any) []string {
	var out []string
	add := func(v any) {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	add(f["resource"])
	add(f["computation"])
	for _, key := range []string{"executor", "attester"} {
		if m, ok := f[key].(map[string]any); ok {
			add(m["resource"])
		}
	}
	if sources, ok := f["sources"].([]any); ok {
		for _, e := range sources {
			if m, ok := e.(map[string]any); ok {
				add(m["resource"])
			}
		}
	}
	return out
}
