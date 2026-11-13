package cli

import (
	"encoding/json"
	"io"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/text/width"

	"github.com/t-kitabayashi-churadata/okf-lens/internal/query"
	"github.com/t-kitabayashi-churadata/okf-lens/internal/source"
)

// displayWidth は端末での表示幅を返す。全角文字は 2 とする。
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		switch width.LookupRune(r).Kind() {
		case width.EastAsianWide, width.EastAsianFullwidth:
			w += 2
		default:
			w++
		}
	}
	return w
}

// cell は表のセルに入れられるよう、改行とタブを空白に置き換える。
func cell(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		return r
	}, s)
}

// writeTable は先頭行をヘッダーとして、列を揃えて書く。最後の列は埋めない。
func writeTable(w io.Writer, rows [][]string) error {
	if len(rows) == 0 {
		return nil
	}
	cols := len(rows[0])
	widths := make([]int, cols)
	for _, row := range rows {
		for i := 0; i < cols-1; i++ {
			if n := displayWidth(row[i]); n > widths[i] {
				widths[i] = n
			}
		}
	}
	var b strings.Builder
	for _, row := range rows {
		for i, c := range row {
			b.WriteString(c)
			if i < cols-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-displayWidth(c)+2))
			}
		}
		b.WriteByte('\n')
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func writeSearchTable(w io.Writer, matches []query.Match) error {
	if len(matches) == 0 {
		return nil
	}
	rows := [][]string{{"TYPE", "TITLE", "PATH"}}
	for _, m := range matches {
		rows = append(rows, []string{cell(m.Doc.Type), cell(m.Doc.Title), cell(m.Doc.Path)})
	}
	return writeTable(w, rows)
}

type searchItem struct {
	Path        string   `json:"path"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Aliases     []string `json:"aliases"`
	Score       int      `json:"score"`
}

func writeSearchJSON(w io.Writer, matches []query.Match) error {
	items := make([]searchItem, 0, len(matches))
	for _, m := range matches {
		items = append(items, searchItem{
			Path:        m.Doc.Path,
			Type:        m.Doc.Type,
			Title:       m.Doc.Title,
			Description: m.Doc.Description,
			Tags:        nonNil(m.Doc.Tags),
			Aliases:     nonNil(m.Doc.Aliases),
			Score:       m.Score,
		})
	}
	return writeJSON(w, items)
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

type typeCount struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// countTypes は type の表記ごとに件数を数える。
// 検索は大文字小文字を区別しないが、表記揺れ(API と api)を見つけるため、表記ごとに分けて数える。
func countTypes(res *source.Result) []typeCount {
	m := map[string]int{}
	for _, d := range res.Docs {
		m[d.Type]++
	}
	out := make([]typeCount, 0, len(m))
	for t, n := range m {
		out = append(out, typeCount{Type: t, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Type < out[j].Type
	})
	return out
}

func writeTypesTable(w io.Writer, counts []typeCount) error {
	if len(counts) == 0 {
		return nil
	}
	rows := [][]string{{"TYPE", "COUNT"}}
	for _, c := range counts {
		rows = append(rows, []string{cell(c.Type), strconv.Itoa(c.Count)})
	}
	return writeTable(w, rows)
}

func writeTypesJSON(w io.Writer, counts []typeCount) error {
	if counts == nil {
		counts = []typeCount{}
	}
	return writeJSON(w, counts)
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
