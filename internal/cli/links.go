package cli

import (
	"fmt"
	"io"

	"github.com/t-kitabayashi-churadata/okf-lens/internal/graph"
	"github.com/t-kitabayashi-churadata/okf-lens/internal/okf"
)

// filterByLinks は --links-to と --linked-from の条件で文書を絞る。複数指定は AND。
// どちらも指定がなければ、リンクを解析せず docs をそのまま返す。
//
// 指定した文書が存在しなくてもエラーにしない。OKF 仕様は壊れたリンクの許容を求めており、
// 存在しない文書へのリンク元を探す使い方があるため。存在しないときは注意を出す。
func filterByLinks(docs []*okf.Document, linksTo, linkedFrom []string, stderr io.Writer) []*okf.Document {
	if len(linksTo) == 0 && len(linkedFrom) == 0 {
		return docs
	}
	g := graph.Build(docs)

	var allowed map[string]bool // nil はまだ絞っていない状態。値は文書のパス
	narrow := func(set map[string]bool) {
		if allowed == nil {
			allowed = set
			return
		}
		for p := range allowed {
			if !set[p] {
				delete(allowed, p)
			}
		}
	}

	for _, t := range linksTo {
		key := graph.ParseTarget(t)
		if !g.Has(key) {
			fmt.Fprintf(stderr, "注意: --links-to の対象が存在しない: %s(壊れたリンクの参照元は検索できる)\n", t)
		}
		set := map[string]bool{}
		for _, p := range g.Backlinks(key) {
			set[p] = true
		}
		narrow(set)
	}
	for _, s := range linkedFrom {
		key := graph.ParseTarget(s)
		if !g.Has(key) {
			fmt.Fprintf(stderr, "注意: --linked-from の対象が存在しない: %s\n", s)
		}
		set := map[string]bool{}
		for _, k := range g.Outlinks(key) {
			// リンク先が存在しない(壊れたリンク)ものは、検索結果にできないので含めない。
			if p, ok := g.Path(k); ok {
				set[p] = true
			}
		}
		narrow(set)
	}

	var out []*okf.Document
	for _, d := range docs {
		if allowed[d.Path] {
			out = append(out, d)
		}
	}
	return out
}
