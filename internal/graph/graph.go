// Package graph は concept 間のリンクを解決し、逆引き表を作る。
package graph

import (
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/t-kitabayashi-churadata/okf-lens/internal/okf"
)

// Graph は concept 間のリンクの有向グラフ。
// ノードは Key で表す(NFC、小文字、区切りは "/")。
// リンク先が存在しない壊れたリンクも保持する(OKF 仕様は壊れたリンクの許容を求める)。
type Graph struct {
	docs map[string]string   // Key → 文書のパス
	out  map[string][]string // リンク元の Key → リンク先の Key(重複なし、昇順)
	in   map[string][]string // リンク先の Key → リンク元の文書パス(重複なし、昇順)
}

// Build は docs のリンクを解決してグラフを作る。
// 自己リンクと、同じ宛先への重複は1つにまとめる。
func Build(docs []*okf.Document) *Graph {
	g := &Graph{
		docs: make(map[string]string, len(docs)),
		out:  map[string][]string{},
		in:   map[string][]string{},
	}
	for _, d := range docs {
		g.docs[Key(d.Path)] = d.Path
	}
	for _, d := range docs {
		from := Key(d.Path)
		seen := map[string]bool{}
		for _, raw := range okf.ExtractLinks(d) {
			to, ok := Resolve(d.Path, raw)
			if !ok || to == from || seen[to] {
				continue
			}
			seen[to] = true
			g.out[from] = append(g.out[from], to)
			g.in[to] = append(g.in[to], d.Path)
		}
	}
	for _, v := range g.out {
		sort.Strings(v)
	}
	for _, v := range g.in {
		sort.Strings(v)
	}
	return g
}

// Has は key の文書が存在するかを返す。
func (g *Graph) Has(key string) bool {
	_, ok := g.docs[key]
	return ok
}

// Path は key の文書のパスを返す。存在しなければ ok が false。
func (g *Graph) Path(key string) (p string, ok bool) {
	p, ok = g.docs[key]
	return p, ok
}

// Backlinks は target(Key)にリンクしている文書のパスを返す。
// target が存在しない文書でもよい。
func (g *Graph) Backlinks(target string) []string { return g.in[target] }

// Outlinks は source(Key)がリンクしている先の Key を返す。
// 存在しない文書への壊れたリンクも含む。
func (g *Graph) Outlinks(source string) []string { return g.out[source] }

// Key は文書のパスを比較用の形にする。
func Key(p string) string {
	return strings.ToLower(norm.NFC.String(strings.ReplaceAll(p, `\`, "/")))
}

// ParseTarget は利用者が指定したパス(--links-to など)を Key にする。
// 先頭の "./" と "/"、".md" の省略、"\" 区切り、大文字小文字の違いを許容する。
func ParseTarget(s string) string {
	p := path.Clean(strings.ReplaceAll(strings.TrimSpace(s), `\`, "/"))
	p = strings.TrimLeft(p, "/")
	if !strings.EqualFold(path.Ext(p), ".md") {
		p += ".md"
	}
	return Key(p)
}

var scheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.\-]*:`)

// Resolve は、リンク元 from(検索ルートからの相対パス)に書かれたリンクの宛先 target を、
// リンク先の Key に解決する。concept へのリンクと解釈できないときは ok が false。
//
//   - "/" 始まりはバンドルルートからのパス
//   - それ以外は from のあるディレクトリ基準の相対パス(frontmatter のパス値も同じ)
//   - URL(スキーム付き、"//" 始まり)、"#見出し" だけのリンク、バンドルの外を指すパスは対象外
//   - "#見出し" と "?query" は除去し、パーセントエンコードは復号する
//   - ".md" の省略を許容する。ディレクトリと、".md" 以外の拡張子は対象外
func Resolve(from, target string) (key string, ok bool) {
	t := strings.TrimSpace(target)
	if t == "" || strings.HasPrefix(t, "//") || scheme.MatchString(t) {
		return "", false
	}
	if i := strings.IndexAny(t, "#?"); i >= 0 {
		t = t[:i]
	}
	if t == "" || strings.HasSuffix(t, "/") {
		return "", false
	}
	if dec, err := url.PathUnescape(t); err == nil {
		t = dec
	}

	var p string
	if strings.HasPrefix(t, "/") {
		p = path.Clean(t[1:])
	} else {
		p = path.Join(path.Dir(strings.ReplaceAll(from, `\`, "/")), t)
	}
	if p == "." || p == ".." || strings.HasPrefix(p, "../") {
		return "", false
	}
	switch ext := path.Ext(p); {
	case strings.EqualFold(ext, ".md"):
	case ext == "":
		p += ".md"
	default:
		return "", false
	}
	return Key(p), true
}
