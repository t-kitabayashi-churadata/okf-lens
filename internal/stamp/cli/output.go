package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"golang.org/x/text/width"

	"github.com/t-kitabayashi-churadata/okf-lens/internal/stamp"
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

// detail は table の DETAIL 列の内容(設計書 9.1 節)。
func detail(f stamp.FileResult) string {
	var parts []string
	switch f.Action {
	case stamp.ActionSkip:
		var msgs []string
		for _, e := range f.Errors {
			msgs = append(msgs, e.Message)
		}
		parts = append(parts, strings.Join(msgs, "; "))
	case "add", "update":
		var ch []string
		for _, k := range f.Added {
			ch = append(ch, "+"+k)
		}
		for _, c := range f.Changes {
			if c.Before != nil {
				ch = append(ch, "~"+c.Key+"("+short(c.Before)+"→"+short(c.After)+")")
			}
		}
		s := strings.Join(ch, " ")
		if len(f.Kept) > 0 {
			s += " (kept: " + strings.Join(f.Kept, ", ") + ")"
		}
		parts = append(parts, s)
	}
	for _, w := range f.Warnings {
		parts = append(parts, "warn: "+w.Code)
	}
	return strings.Join(parts, " ")
}

func writeTable(w io.Writer, rep *stamp.Report) error {
	if len(rep.Files) == 0 {
		return nil
	}
	rows := [][]string{{"ACTION", "PATH", "DETAIL"}}
	for _, f := range rep.Files {
		rows = append(rows, []string{f.Action, cell(f.Path), cell(detail(f))})
	}
	widths := make([]int, 2)
	for _, row := range rows {
		for i := 0; i < 2; i++ {
			if n := displayWidth(row[i]); n > widths[i] {
				widths[i] = n
			}
		}
	}
	var b strings.Builder
	for _, row := range rows {
		for i, c := range row {
			b.WriteString(c)
			if i < 2 {
				b.WriteString(strings.Repeat(" ", widths[i]-displayWidth(c)+2))
			}
		}
		b.WriteByte('\n')
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func writeJSON(w io.Writer, rep *stamp.Report) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

// writeSummary は件数と、dry-run の注意を標準エラー出力に書く。
func writeSummary(w io.Writer, rep *stamp.Report, verbose bool) {
	s := rep.Summary
	fmt.Fprintf(w, "%d 件(add %d、update %d、unchanged %d、skip %d)\n",
		len(rep.Files), s["add"], s["update"], s["unchanged"], s["skip"])
	if rep.DryRun {
		fmt.Fprintln(w, "dry-run: ファイルは変更していません。--write で反映します")
	}
	if verbose {
		for _, f := range rep.Files {
			for _, e := range f.Errors {
				fmt.Fprintf(w, "エラー %s: [%s] %s\n", f.Path, e.Code, e.Message)
			}
			for _, e := range f.Warnings {
				fmt.Fprintf(w, "警告 %s: [%s] %s\n", f.Path, e.Code, e.Message)
			}
		}
	}
}

// short は table に出す値を、1行に収まる長さの文字列にする。
func short(v any) string {
	var s string
	switch x := v.(type) {
	case string:
		s = x
	default:
		b, err := json.Marshal(x)
		if err != nil {
			s = fmt.Sprint(x)
		} else {
			s = string(b)
		}
	}
	if r := []rune(s); len(r) > 30 {
		s = string(r[:30]) + "…"
	}
	return s
}

func writeJSONValue(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// writeRows は先頭行をヘッダーとして、列を揃えて書く。最後の列は埋めない。
func writeRows(w io.Writer, rows [][]string) error {
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
