package stamp

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/t-kitabayashi-churadata/okf-lens/internal/okf"
)

// Action は1ファイルに対する結果。
type Action string

const (
	ActionAdd       Action = "add"       // frontmatter を新規に作る
	ActionUpdate    Action = "update"    // 既存の frontmatter を変更する
	ActionUnchanged Action = "unchanged" // 変更なし
)

// KeyChange は1つのキーの変更。新しく書いたキーは Before が nil。
type KeyChange struct {
	Key    string `json:"key"`
	Before any    `json:"before,omitempty"`
	After  any    `json:"after"`
}

// Change は Apply の結果。
type Change struct {
	Action Action
	// Added は新しく書いたキー。
	Added []string
	// Overwritten は既存の値を置き換えたキー(merge で要素を足したキーを含む)。
	Overwritten []string
	// Kept は入力にあったが、既存の値を残したキー。
	Kept []string
	// Changes は書いたキーごとの、変更前と変更後の値(Added と Overwritten のキー)。
	Changes []KeyChange
	// New は書き換え後の内容。Action が unchanged のときは nil。
	New []byte
}

// listKeys は merge で和集合にするキー。
var listKeys = map[string]bool{"tags": true, "aliases": true}

type edit struct {
	start, end int
	text       string
}

// Apply は data(ファイルの内容)に、検証済みの input を mode に従って適用する。
//
// 既存の frontmatter は YAML に再エンコードしない。キーの追加は閉じ側の "---" の
// 直前への差し込み、上書きは該当キーの行範囲の置き換えで行い、ほかのバイトは変えない。
// 本文も変えない。BOM と改行(LF、CRLF)はファイルのものを保つ。
//
// 組み立てた内容は okf.Parse で検証する。失敗したときはエラーを返し、
// 呼び出し側は何も書かない。
func Apply(path string, data []byte, input map[string]any, mode Mode) (*Change, []*Error) {
	if !utf8.Valid(data) {
		return nil, []*Error{errf(CodeNotUTF8, "UTF-8 ではない")}
	}

	parts, err := okf.SplitRaw(data)
	switch {
	case errors.Is(err, okf.ErrNoFrontmatter):
		return applyNew(path, data, parts, input, mode)
	case err != nil:
		return nil, []*Error{errf(CodeBrokenFrontmatter, err.Error())}
	}

	front := data[parts.FrontStart:parts.FrontEnd]
	pairs, perr := rootPairs(front)
	if perr != nil {
		return nil, []*Error{perr}
	}
	starts := lineStarts(front)

	existing := map[string]int{} // キー → pairs の添字
	for i, p := range pairs {
		k := p.key.Value
		if _, dup := existing[k]; !dup {
			existing[k] = i
		}
	}

	ch := &Change{}
	var edits []edit
	var appended strings.Builder

	for _, k := range orderedKeys(input) {
		v := input[k]
		i, exists := existing[k]
		if !exists {
			text, e := renderKey(k, v, parts.Newline)
			if e != nil {
				return nil, []*Error{errf(CodeInvalidValue, e.Error())}
			}
			appended.WriteString(text)
			ch.Added = append(ch.Added, k)
			ch.Changes = append(ch.Changes, KeyChange{Key: k, After: v})
			continue
		}
		if mode == ModeFill {
			ch.Kept = append(ch.Kept, k)
			continue
		}

		newVal := v
		if mode == ModeMerge {
			if !listKeys[k] {
				ch.Kept = append(ch.Kept, k)
				continue
			}
			merged, changed, e := mergeList(k, pairs[i].val, v)
			if e != nil {
				return nil, []*Error{e}
			}
			if !changed {
				ch.Kept = append(ch.Kept, k)
				continue
			}
			newVal = merged
		}

		if sameValue(pairs[i].val, newVal) {
			ch.Kept = append(ch.Kept, k)
			continue
		}
		text, e := renderKey(k, newVal, parts.Newline)
		if e != nil {
			return nil, []*Error{errf(CodeInvalidValue, e.Error())}
		}
		s, en := keyRange(front, starts, pairs, i)
		edits = append(edits, edit{start: s, end: en, text: text})
		ch.Overwritten = append(ch.Overwritten, k)
		var before any
		_ = pairs[i].val.Decode(&before)
		ch.Changes = append(ch.Changes, KeyChange{Key: k, Before: before, After: newVal})
	}

	if len(edits) == 0 && appended.Len() == 0 {
		ch.Action = ActionUnchanged
		return verified(ch, finalCheck(path, data, input, ch, mode, true))
	}

	// 後ろの編集から適用して、前のオフセットがずれないようにする。
	sort.Slice(edits, func(a, b int) bool { return edits[a].start > edits[b].start })
	newFront := append([]byte(nil), front...)
	for _, e := range edits {
		newFront = append(newFront[:e.start:e.start], append([]byte(e.text), newFront[e.end:]...)...)
	}
	newFront = append(newFront, appended.String()...)

	var out bytes.Buffer
	out.Write(data[:parts.FrontStart])
	out.Write(newFront)
	out.Write(data[parts.FrontEnd:])

	ch.Action = ActionUpdate
	ch.New = out.Bytes()
	return verified(ch, finalCheck(path, ch.New, input, ch, mode, false))
}

// applyNew は frontmatter がないファイルに、新しい frontmatter を作る。
func applyNew(path string, data []byte, parts okf.RawParts, input map[string]any, mode Mode) (*Change, []*Error) {
	var fm strings.Builder
	ch := &Change{Action: ActionAdd}
	for _, k := range orderedKeys(input) {
		text, e := renderKey(k, input[k], parts.Newline)
		if e != nil {
			return nil, []*Error{errf(CodeInvalidValue, e.Error())}
		}
		fm.WriteString(text)
		ch.Added = append(ch.Added, k)
		ch.Changes = append(ch.Changes, KeyChange{Key: k, After: input[k]})
	}

	body := data
	var out bytes.Buffer
	if parts.BOM {
		out.Write(data[:len("\xEF\xBB\xBF")])
		body = data[len("\xEF\xBB\xBF"):]
	}
	out.WriteString("---" + parts.Newline)
	out.WriteString(fm.String())
	out.WriteString("---" + parts.Newline)
	if len(body) > 0 {
		out.WriteString(parts.Newline)
		out.Write(body)
	}
	ch.New = out.Bytes()
	return verified(ch, finalCheck(path, ch.New, input, ch, mode, false))
}

// verified は、検証エラーがあれば Change を捨てる。エラーの内容は書かせない。
func verified(ch *Change, errs []*Error) (*Change, []*Error) {
	if errs != nil {
		return nil, errs
	}
	return ch, nil
}

// finalCheck は、書き換え後の内容を okf.Parse で検証する。
// 検証に失敗した内容は書かない。
func finalCheck(path string, content []byte, input map[string]any, ch *Change, mode Mode, unchanged bool) []*Error {
	doc, err := okf.Parse(path, content)
	switch {
	case errors.Is(err, okf.ErrMissingType):
		return []*Error{errf(CodeMissingType, "type がない")}
	case errors.Is(err, okf.ErrInvalidType):
		return []*Error{errf(CodeInvalidValue, "既存の type が文字列ではない")}
	case err != nil:
		return []*Error{errf(CodeVerifyFailed, "書き換え後の内容を読めない: "+err.Error())}
	}
	if unchanged {
		return nil
	}

	written := map[string]bool{}
	for _, k := range ch.Added {
		written[k] = true
	}
	for _, k := range ch.Overwritten {
		written[k] = true
	}
	check := func(key string, got any, want any) *Error {
		if written[key] && !reflect.DeepEqual(got, want) {
			return errf(CodeVerifyFailed, fmt.Sprintf("%s が入力どおりに読めない: %v", key, got))
		}
		return nil
	}
	if e := check("type", doc.Type, input["type"]); e != nil && input["type"] != nil {
		return []*Error{e}
	}
	// merge は既存の要素を含むので、入力の値だけで書いたキーだけを厳密に比べる。
	if tags, ok := input["tags"].([]string); ok && onlyInput(ch, mode, "tags") {
		if e := check("tags", nonNil(doc.Tags), tags); e != nil {
			return []*Error{e}
		}
	}
	if al, ok := input["aliases"].([]string); ok && onlyInput(ch, mode, "aliases") {
		if e := check("aliases", nonNil(doc.Aliases), al); e != nil {
			return []*Error{e}
		}
	}
	return nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// onlyInput は、key を入力の値だけで書いた(新規追加、または overwrite での置き換え)
// ときに true を返す。merge で要素を足したキーは、既存の要素を含むので false。
func onlyInput(ch *Change, mode Mode, key string) bool {
	for _, k := range ch.Added {
		if k == key {
			return true
		}
	}
	if mode == ModeOverwrite {
		for _, k := range ch.Overwritten {
			if k == key {
				return true
			}
		}
	}
	return false
}

type pair struct{ key, val *yaml.Node }

// rootPairs は frontmatter の YAML のトップレベルのキーと値を、出現順に返す。
func rootPairs(front []byte) ([]pair, *Error) {
	if len(bytes.TrimSpace(front)) == 0 {
		return nil, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(front, &doc); err != nil {
		return nil, errf(CodeBrokenFrontmatter, "frontmatter の YAML が不正: "+err.Error())
	}
	if doc.Kind == 0 { // コメントだけ
		return nil, nil
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errf(CodeBrokenFrontmatter, "frontmatter がキーと値の組ではない")
	}
	m := doc.Content[0]
	out := make([]pair, 0, len(m.Content)/2)
	for i := 0; i+1 < len(m.Content); i += 2 {
		out = append(out, pair{key: m.Content[i], val: m.Content[i+1]})
	}
	return out, nil
}

// lineStarts は各行の先頭のオフセットを返す(行番号 n は starts[n-1])。
func lineStarts(b []byte) []int {
	starts := []int{}
	if len(b) == 0 {
		return starts
	}
	starts = append(starts, 0)
	for i, c := range b {
		if c == '\n' && i+1 < len(b) {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// keyRange は pairs[i] のキーの行範囲を、front 内のバイトオフセットで返す。
// 次のキーの手前までを範囲とし、末尾の空行とコメント行は範囲から外して残す。
func keyRange(front []byte, starts []int, pairs []pair, i int) (int, int) {
	first := pairs[i].key.Line
	next := len(starts) + 1
	if i+1 < len(pairs) {
		next = pairs[i+1].key.Line
	}
	last := next - 1
	for last > first {
		line := strings.TrimSpace(string(lineAt(front, starts, last)))
		if line == "" || strings.HasPrefix(line, "#") {
			last--
			continue
		}
		break
	}
	end := len(front)
	if last < len(starts) {
		end = starts[last]
	}
	return starts[first-1], end
}

func lineAt(b []byte, starts []int, n int) []byte {
	s := starts[n-1]
	e := len(b)
	if n < len(starts) {
		e = starts[n]
	}
	return b[s:e]
}

// renderKey は1つのキーを YAML の文字列にし、改行を nl にそろえる。
func renderKey(key string, v any, nl string) (string, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(map[string]any{key: v}); err != nil {
		return "", fmt.Errorf("%s を YAML にできない: %w", key, err)
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	s := buf.String()
	if nl != "\n" {
		s = strings.ReplaceAll(s, "\n", nl)
	}
	return s, nil
}

// sameValue は既存の値と入力が同じ内容かを返す。同じ書式で出力して比べる。
func sameValue(node *yaml.Node, v any) bool {
	var cur any
	if err := node.Decode(&cur); err != nil {
		return false
	}
	a, err1 := renderKey("k", cur, "\n")
	b, err2 := renderKey("k", v, "\n")
	return err1 == nil && err2 == nil && a == b
}

// mergeList は既存のリストに、入力の要素のうち無いものを足す。
// 足す要素がなければ changed は false。
func mergeList(key string, node *yaml.Node, add any) (merged []string, changed bool, e *Error) {
	var cur any
	if err := node.Decode(&cur); err != nil {
		return nil, false, errf(CodeInvalidValue, "既存の "+key+" を読めない")
	}
	var have []string
	switch x := cur.(type) {
	case nil:
	case string:
		for _, s := range strings.Split(x, ",") {
			if s = strings.TrimSpace(s); s != "" {
				have = append(have, s)
			}
		}
	case []any:
		for _, el := range x {
			s, ok := el.(string)
			if !ok {
				return nil, false, errf(CodeInvalidValue, "既存の "+key+" に文字列でない要素がある")
			}
			have = append(have, s)
		}
	default:
		return nil, false, errf(CodeInvalidValue, "既存の "+key+" をリストとして読めない")
	}

	seen := map[string]bool{}
	for _, s := range have {
		seen[fold(strings.TrimPrefix(s, "#"))] = true
	}
	merged = append([]string(nil), have...)
	for _, s := range add.([]string) {
		if !seen[fold(s)] {
			seen[fold(s)] = true
			merged = append(merged, s)
			changed = true
		}
	}
	return merged, changed, nil
}
