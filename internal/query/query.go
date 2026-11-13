// Package query は concept の検索条件を評価し、結果を並べる。
package query

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/t-kitabayashi-churadata/okf-lens/internal/okf"
)

// Query は検索条件。異なる種類の条件は AND で結合する。
// すべての比較で大文字小文字を区別しない。
type Query struct {
	// Text は title、aliases、description への部分一致。
	// 空白で区切った語は、すべてを含む文書だけが一致する(AND)。
	Text string
	// Tags はすべてを持つ文書だけが一致する(AND)。
	// "設計" は "設計" と "設計/基本設計" に一致する(階層一致)。
	Tags []string
	// Types はいずれかに一致する文書が対象(OR)。
	Types []string
	// Where は任意のトップレベルキーの一致条件。すべてを満たす文書が対象(AND)。
	// リストの値は、要素のいずれかに一致すればよい。
	Where map[string]string
}

// Match は検索に一致した文書とそのスコア。
type Match struct {
	Doc   *okf.Document
	Score int
}

// 一致した場所ごとのスコア。語ごとに最も高い1つを採用して合計する。
const (
	scoreTitleExact    = 100
	scoreTitleContains = 60
	scoreAlias         = 40
	scoreDescription   = 20
)

// defaultStatus は status を省略した文書に適用する値(OKF v0.2)。
const defaultStatus = "stable"

// Search は docs から q に一致する文書を返す。
// Text があるときはスコアの降順、同点または Text がないときはパスの辞書順。
func Search(docs []*okf.Document, q Query) []Match {
	c := compile(q)
	var out []Match
	for _, d := range docs {
		if score, ok := c.match(d); ok {
			out = append(out, Match{Doc: d, Score: score})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Doc.Path < out[j].Doc.Path
	})
	return out
}

// compiled は正規化済みの検索条件。
type compiled struct {
	terms []string
	tags  []string
	types []string
	where []cond
}

type cond struct{ key, value string }

func compile(q Query) compiled {
	var c compiled
	for _, t := range strings.Fields(fold(q.Text)) {
		c.terms = append(c.terms, t)
	}
	for _, t := range q.Tags {
		if t = strings.TrimPrefix(fold(strings.TrimSpace(t)), "#"); t != "" {
			c.tags = append(c.tags, t)
		}
	}
	for _, t := range q.Types {
		if t = fold(strings.TrimSpace(t)); t != "" {
			c.types = append(c.types, t)
		}
	}
	// map の走査順に依存しないよう、キー順に並べる。
	keys := make([]string, 0, len(q.Where))
	for k := range q.Where {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		c.where = append(c.where, cond{key: fold(strings.TrimSpace(k)), value: fold(strings.TrimSpace(q.Where[k]))})
	}
	return c
}

// fold は NFC 正規化と小文字化を行う。
func fold(s string) string { return strings.ToLower(norm.NFC.String(s)) }

func (c compiled) match(d *okf.Document) (int, bool) {
	if len(c.types) > 0 && !matchType(d.Type, c.types) {
		return 0, false
	}
	for _, t := range c.tags {
		if !hasTag(d.Tags, t) {
			return 0, false
		}
	}
	for _, w := range c.where {
		if !matchWhere(d, w) {
			return 0, false
		}
	}
	return scoreText(d, c.terms)
}

func matchType(docType string, types []string) bool {
	dt := fold(docType)
	for _, t := range types {
		if dt == t {
			return true
		}
	}
	return false
}

// hasTag は階層一致を含めて、タグを持つかを返す。
func hasTag(docTags []string, tag string) bool {
	for _, dt := range docTags {
		dt = fold(dt)
		if dt == tag || strings.HasPrefix(dt, tag+"/") {
			return true
		}
	}
	return false
}

// matchWhere は任意キーの一致を返す。
// status を省略した文書は、"stable" として扱う。
func matchWhere(d *okf.Document, w cond) bool {
	found := false
	for k, v := range d.Fields {
		if fold(k) != w.key {
			continue
		}
		found = true
		if valueMatches(v, w.value) {
			return true
		}
	}
	if !found && w.key == "status" {
		return w.value == defaultStatus
	}
	return false
}

// valueMatches はスカラ、またはスカラのリストが want と一致するかを返す。
// ネストしたマップは対象外。
func valueMatches(v any, want string) bool {
	switch x := v.(type) {
	case []any:
		for _, e := range x {
			if scalarString(e) == want {
				return true
			}
		}
		return false
	default:
		return scalarString(v) == want
	}
}

// scalarString はスカラを比較用の文字列にする。スカラ以外は一致しない値を返す。
func scalarString(v any) string {
	switch x := v.(type) {
	case string:
		return fold(x)
	case bool, int, int64, uint64, float64:
		return fold(fmt.Sprint(x))
	case time.Time:
		if x.Hour() == 0 && x.Minute() == 0 && x.Second() == 0 && x.Nanosecond() == 0 {
			return x.Format("2006-01-02")
		}
		return fold(x.Format(time.RFC3339))
	default:
		// nil、マップなど。空文字列の条件には一致させない。
		return "\x00"
	}
}

// scoreText は Text 条件を評価する。条件がなければスコア 0 で一致とする。
func scoreText(d *okf.Document, terms []string) (int, bool) {
	if len(terms) == 0 {
		return 0, true
	}
	title := fold(d.Title)
	desc := fold(d.Description)
	aliases := make([]string, len(d.Aliases))
	for i, a := range d.Aliases {
		aliases[i] = fold(a)
	}

	total := 0
	for _, t := range terms {
		best := 0
		switch {
		case title == t:
			best = scoreTitleExact
		case strings.Contains(title, t):
			best = scoreTitleContains
		case anyContains(aliases, t):
			best = scoreAlias
		case strings.Contains(desc, t):
			best = scoreDescription
		}
		if best == 0 {
			return 0, false
		}
		total += best
	}
	return total, true
}

func anyContains(list []string, sub string) bool {
	for _, s := range list {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
