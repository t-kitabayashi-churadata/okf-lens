// Package okf は OKF(Open Knowledge Format)の concept ファイルを読み込む。
package okf

// Document は1つの concept ファイルを表す。
// 文字列はすべて NFC に正規化済み。
type Document struct {
	// Path は検索ルートからの相対パス(区切りは常に "/")。
	Path string
	// Type は frontmatter の type。必須。
	Type string
	// Title は frontmatter の title、本文の最初の H1、ファイル名の順で決まる。
	Title string
	// Description は frontmatter の description。
	Description string
	// Tags は検証済みのタグ。先頭の "#" は除去済み。
	Tags []string
	// Aliases は別名。
	Aliases []string
	// Fields は frontmatter の全キー。未知のキーも保持する。
	Fields map[string]any
	// Body は frontmatter を除いた本文。改行は LF に統一済み。
	Body string
	// Warnings は読み飛ばした不正なタグなど、致命的でない問題。
	Warnings []string
}
