package okf

import (
	"errors"
	"testing"
)

func TestSplitRaw(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want RawParts
		err  error
	}{
		{
			name: "LF",
			in:   "---\ntype: API\n---\n本文\n",
			want: RawParts{Newline: "\n", FrontStart: 4, FrontEnd: 14, BodyStart: 18},
		},
		{
			name: "CRLF",
			in:   "---\r\ntype: API\r\n---\r\n本文\r\n",
			want: RawParts{Newline: "\r\n", FrontStart: 5, FrontEnd: 16, BodyStart: 21},
		},
		{
			name: "BOM",
			in:   "\xEF\xBB\xBF---\ntype: API\n---\n",
			want: RawParts{BOM: true, Newline: "\n", FrontStart: 7, FrontEnd: 17, BodyStart: 21},
		},
		{
			name: "空の frontmatter",
			in:   "---\n---\n本文",
			want: RawParts{Newline: "\n", FrontStart: 4, FrontEnd: 4, BodyStart: 8},
		},
		{
			name: "終端の改行なし",
			in:   "---\na: 1\n---",
			want: RawParts{Newline: "\n", FrontStart: 4, FrontEnd: 9, BodyStart: 12},
		},
		{
			name: "フェンスの行末の空白",
			in:   "---  \na: 1\n---\t\n",
			want: RawParts{Newline: "\n", FrontStart: 6, FrontEnd: 11, BodyStart: 16},
		},
		{name: "frontmatter なし", in: "# 見出し\n", err: ErrNoFrontmatter},
		{name: "空ファイル", in: "", err: ErrNoFrontmatter},
		{name: "終端なし", in: "---\ntype: API\n本文\n", err: ErrUnterminatedFrontmatter},
		{name: "CR だけの改行", in: "---\rtype: API\r---\r", err: ErrLoneCR},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SplitRaw([]byte(tt.in))
			if !errors.Is(err, tt.err) {
				t.Fatalf("err = %v, want %v", err, tt.err)
			}
			if tt.err == nil && got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// Parse と SplitRaw は、frontmatter の有無について同じ判定をする。
func TestSplitRaw_agreesWithParse(t *testing.T) {
	inputs := []string{
		"---\ntype: API\n---\n本文",
		"---\r\ntype: API\r\n---\r\n本文",
		"# 見出し",
		"---\ntype: API\n本文",
		"\xEF\xBB\xBF---\ntype: API\n---\n",
	}
	for _, in := range inputs {
		_, rawErr := SplitRaw([]byte(in))
		_, parseErr := Parse("a.md", []byte(in))
		if errors.Is(parseErr, ErrNoFrontmatter) != errors.Is(rawErr, ErrNoFrontmatter) ||
			errors.Is(parseErr, ErrUnterminatedFrontmatter) != errors.Is(rawErr, ErrUnterminatedFrontmatter) {
			t.Errorf("%q: Parse=%v SplitRaw=%v", in, parseErr, rawErr)
		}
	}
}
