// Package cli は okf-lens のコマンドライン処理を行う。
// 引数の解析と出力の整形だけを担い、検索は internal/query に任せる。
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/t-kitabayashi-churadata/okf-lens/internal/query"
	"github.com/t-kitabayashi-churadata/okf-lens/internal/source"
)

// 終了コード。ヒット 0 件は正常終了(0)とする。
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

// Version はビルド時に -ldflags で設定する。設定しない開発ビルドは "dev"。
var Version = "dev"

// Run はコマンドを実行し、終了コードを返す。
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return exitUsage
	}
	switch args[0] {
	case "search":
		return runSearch(args[1:], stdout, stderr)
	case "types":
		return runTypes(args[1:], stdout, stderr)
	case "version", "-v", "--version":
		fmt.Fprintf(stdout, "okf-lens %s\n", Version)
		return exitOK
	case "help", "-h", "--help":
		usage(stdout)
		return exitOK
	default:
		fmt.Fprintf(stderr, "不明なコマンド: %s\n\n", args[0])
		usage(stderr)
		return exitUsage
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `okf-lens: OKF のバンドルを検索する

使い方:
  okf-lens search [QUERY] [フラグ]   文書を検索する
  okf-lens types [フラグ]            使われている type を件数付きで表示する
  okf-lens version                   バージョンを表示する
  okf-lens help                      このヘルプを表示する

各コマンドの詳細は okf-lens <コマンド> -h で表示する。
`)
}

// multiFlag は繰り返し指定できるフラグ。
type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

// common は全コマンドで共通のフラグ。
type common struct {
	dir     *string
	format  *string
	verbose *bool
	exclude *multiFlag
}

func newFlagSet(name, synopsis string, stderr io.Writer) (*flag.FlagSet, common) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	c := common{
		dir:     fs.String("dir", ".", "検索対象のディレクトリ"),
		format:  fs.String("format", "table", "出力形式(table または json)"),
		verbose: fs.Bool("verbose", false, "読み飛ばしたファイルと警告の詳細を表示する"),
		exclude: new(multiFlag),
	}
	fs.Var(c.exclude, "exclude", "除外するファイルやディレクトリ(gitignore 風)。繰り返し指定できる。検索ルートの .okf-lensignore と合わせて適用する")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "使い方: okf-lens %s\n\nフラグ:\n", synopsis)
		fs.PrintDefaults()
	}
	return fs, c
}

// parseInterspersed は、フラグと位置引数が混在していても解析する。
// 標準の flag パッケージは最初の位置引数で解析を止めるため、繰り返して解析する。
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return positional, nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

// parseResult はフラグ解析の結果を終了コードに変換する。ok が true なら続行する。
func parseResult(err error) (code int, ok bool) {
	switch {
	case err == nil:
		return exitOK, true
	case errors.Is(err, flag.ErrHelp):
		return exitOK, false
	default:
		return exitUsage, false
	}
}

func checkFormat(format string, stderr io.Writer) bool {
	if format == "table" || format == "json" {
		return true
	}
	fmt.Fprintf(stderr, "--format は table または json を指定する: %q\n", format)
	return false
}

// parseWhere は "key=value" の列を検索条件に変換する。同じキーは最後の指定が有効になる。
func parseWhere(items []string) (map[string]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(items))
	for _, it := range items {
		k, v, ok := strings.Cut(it, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			return nil, fmt.Errorf("--where は key=value の形式で指定する: %q", it)
		}
		out[k] = v
	}
	return out, nil
}

func runSearch(args []string, stdout, stderr io.Writer) int {
	fs, c := newFlagSet("search", "search [QUERY] [フラグ]", stderr)
	var tags, types, wheres, linksTo, linkedFrom multiFlag
	fs.Var(&tags, "tag", "タグ。繰り返し指定すると AND。階層一致(設計 は 設計/基本設計 にも一致)")
	fs.Var(&types, "type", "type。繰り返し指定すると OR")
	fs.Var(&wheres, "where", "key=value 形式の任意キー条件。繰り返し指定すると AND")
	fs.Var(&linksTo, "links-to", "このパスの文書にリンクしている文書(backlinks)。.md と先頭の / は省略できる。繰り返し指定すると AND")
	fs.Var(&linkedFrom, "linked-from", "このパスの文書がリンクしている文書。繰り返し指定すると AND")

	positional, err := parseInterspersed(fs, args)
	if code, ok := parseResult(err); !ok {
		return code
	}
	if !checkFormat(*c.format, stderr) {
		return exitUsage
	}
	where, err := parseWhere(wheres)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsage
	}

	res, err := source.ScanDir(*c.dir, source.Options{Exclude: *c.exclude})
	if err != nil {
		return reportScanError(stderr, err)
	}
	reportProblems(stderr, res, *c.verbose)

	docs := filterByLinks(res.Docs, linksTo, linkedFrom, stderr)
	matches := query.Search(docs, query.Query{
		Text:  strings.Join(positional, " "),
		Tags:  tags,
		Types: types,
		Where: where,
	})

	if *c.format == "json" {
		err = writeSearchJSON(stdout, matches)
	} else {
		err = writeSearchTable(stdout, matches)
	}
	if err != nil {
		fmt.Fprintf(stderr, "エラー: %v\n", err)
		return exitError
	}
	fmt.Fprintf(stderr, "%d 件\n", len(matches))
	return exitOK
}

func runTypes(args []string, stdout, stderr io.Writer) int {
	fs, c := newFlagSet("types", "types [フラグ]", stderr)
	positional, err := parseInterspersed(fs, args)
	if code, ok := parseResult(err); !ok {
		return code
	}
	if len(positional) > 0 {
		fmt.Fprintf(stderr, "types は引数を取らない: %q\n", strings.Join(positional, " "))
		return exitUsage
	}
	if !checkFormat(*c.format, stderr) {
		return exitUsage
	}

	res, err := source.ScanDir(*c.dir, source.Options{Exclude: *c.exclude})
	if err != nil {
		return reportScanError(stderr, err)
	}
	reportProblems(stderr, res, *c.verbose)

	counts := countTypes(res)
	if *c.format == "json" {
		err = writeTypesJSON(stdout, counts)
	} else {
		err = writeTypesTable(stdout, counts)
	}
	if err != nil {
		fmt.Fprintf(stderr, "エラー: %v\n", err)
		return exitError
	}
	return exitOK
}

// reportScanError は走査の失敗を表示し、終了コードを返す。
// 除外パターンの書式エラーは、利用者の入力の問題なので引数エラーとして扱う。
func reportScanError(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "エラー: %v\n", err)
	if errors.Is(err, source.ErrInvalidPattern) {
		return exitUsage
	}
	return exitError
}

// reportProblems は読み飛ばしたファイルと警告を標準エラー出力に書く。
// 通常は件数だけ。verbose のときは1件ずつ書く。
func reportProblems(stderr io.Writer, res *source.Result, verbose bool) {
	warnings := 0
	for _, d := range res.Docs {
		warnings += len(d.Warnings)
	}
	if len(res.Skipped) == 0 && warnings == 0 {
		return
	}
	if !verbose {
		fmt.Fprintf(stderr, "%d 件のファイルを読み飛ばした。警告は %d 件(--verbose で詳細を表示)\n", len(res.Skipped), warnings)
		return
	}
	for _, s := range res.Skipped {
		fmt.Fprintf(stderr, "読み飛ばし: %s: %v\n", s.Path, s.Err)
	}
	for _, d := range res.Docs {
		for _, w := range d.Warnings {
			fmt.Fprintf(stderr, "警告: %s: %s\n", d.Path, w)
		}
	}
}
