package okf

import (
	"bytes"
	"errors"
)

// ErrLoneCR は改行が CR だけの行を含むときに返る。
// Parse は CR を改行として扱うが、SplitRaw は CRLF と LF だけを扱う。
var ErrLoneCR = errors.New("okf: CR だけの改行は扱えない")

// RawParts は concept ファイルを、元のバイト列の位置で分けた結果。
// Parse と違い、BOM、改行、YAML の書式を一切変えない。書き換えの用途で使う。
type RawParts struct {
	// BOM は先頭に UTF-8 BOM があるか。BOM は FrontStart より前にある。
	BOM bool
	// Newline はファイルの改行。"\r\n" が1つでもあれば "\r\n"、なければ "\n"。
	Newline string
	// FrontStart は YAML の先頭(開始側の "---" の行の次)のオフセット。
	FrontStart int
	// FrontEnd は終端側の "---" の行の先頭のオフセット。
	// data[FrontStart:FrontEnd] が YAML 本体で、最終行の改行を含む。
	FrontEnd int
	// BodyStart は終端側の "---" の行の次(本文の先頭)のオフセット。
	BodyStart int
}

// SplitRaw は先頭の "---" で囲まれた frontmatter の位置を返す。
// 判定規則は Parse と同じで、"---" の行末の空白と TAB は無視する。
//
// frontmatter がなければ ErrNoFrontmatter、終端がなければ
// ErrUnterminatedFrontmatter を返す。
func SplitRaw(data []byte) (RawParts, error) {
	var p RawParts
	start := 0
	if bytes.HasPrefix(data, utf8BOM) {
		p.BOM = true
		start = len(utf8BOM)
	}
	p.Newline = "\n"
	if bytes.Contains(data, []byte("\r\n")) {
		p.Newline = "\r\n"
	}
	// CRLF の一部ではない CR を検出する。
	for i, b := range data {
		if b == '\r' && (i+1 >= len(data) || data[i+1] != '\n') {
			return p, ErrLoneCR
		}
	}

	first, next := nextLine(data, start)
	if first == nil || !isFence(first) {
		return p, ErrNoFrontmatter
	}
	p.FrontStart = next

	for pos := next; pos < len(data); {
		line, after := nextLine(data, pos)
		if isFence(line) {
			p.FrontEnd = pos
			p.BodyStart = after
			return p, nil
		}
		pos = after
	}
	return p, ErrUnterminatedFrontmatter
}

// nextLine は pos から始まる1行(改行を除く)と、次の行の開始位置を返す。
// pos がデータの末尾以降なら nil を返す。
func nextLine(data []byte, pos int) (line []byte, next int) {
	if pos >= len(data) {
		return nil, len(data)
	}
	i := bytes.IndexByte(data[pos:], '\n')
	if i < 0 {
		return data[pos:], len(data)
	}
	return bytes.TrimSuffix(data[pos:pos+i], []byte("\r")), pos + i + 1
}

func isFence(line []byte) bool {
	return string(bytes.TrimRight(line, " \t")) == "---"
}
