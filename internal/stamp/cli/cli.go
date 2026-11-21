// Package cli は okf-stamp のコマンドライン処理を行う。
// 引数の解析と出力の整形だけを担い、処理は internal/stamp に任せる。
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/t-kitabayashi-churadata/okf-lens/internal/stamp"
)

// 終了コード(設計書 5.4 節)。
const (
	exitOK      = 0
	exitError   = 1 // 実行エラー(ディレクトリがない、書き込み失敗など)
	exitUsage   = 2 // 引数エラー
	exitPartial = 3 // 一部のファイルを入力エラーでスキップした
)

// Version はビルド時に -ldflags で設定する。設定しない開発ビルドは "dev"。
var Version = "dev"

// Run はコマンドを実行し、終了コードを返す。
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return exitUsage
	}
	switch args[0] {
	case "apply":
		return runApply(args[1:], stdin, stdout, stderr)
	case "scan":
		return runScan(args[1:], stdout, stderr)
	case "partition":
		return runPartition(args[1:], stdout, stderr)
	case "schema":
		fmt.Fprint(stdout, stamp.Schema)
		return exitOK
	case "version", "-v", "--version":
		fmt.Fprintf(stdout, "okf-stamp %s\n", Version)
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
	fmt.Fprint(w, `okf-stamp: concept ファイルに OKF の frontmatter を付与する

使い方:
  okf-stamp apply [PATH...] [フラグ]   frontmatter を付与する(既定は dry-run)
  okf-stamp scan [PATH...] [フラグ]    対象ファイルの状態を調べる(変更しない)
  okf-stamp partition [PATH...] [フラグ]  対象を重ならないグループに分ける
  okf-stamp schema                     マニフェストの JSON Schema を表示する
  okf-stamp version                    バージョンを表示する
  okf-stamp help                       このヘルプを表示する

各コマンドの詳細は okf-stamp <コマンド> -h で表示する。
`)
}

// multiFlag は繰り返し指定できるフラグ。
type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

// parseInterspersed は、フラグと位置引数が混在していても解析する。
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

func runApply(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		manifest               = fs.String("manifest", "", "マニフェストの JSON ファイル。- で標準入力")
		typ                    = fs.String("type", "", "全対象に共通の type")
		status                 = fs.String("status", "", "draft、stable、deprecated のいずれか")
		mode                   = fs.String("mode", "fill", "既存の値の扱い(fill、overwrite、merge)。マニフェストの mode が優先する")
		write                  = fs.Bool("write", false, "ファイルに書き込む。指定しないときは dry-run")
		noRecurse              = fs.Bool("no-recurse", false, "ディレクトリの直下のファイルだけを対象にする")
		format                 = fs.String("format", "table", "出力形式(table または json)")
		verbose                = fs.Bool("verbose", false, "警告とエラーのメッセージを1件ずつ表示する")
		tags, aliases, exclude multiFlag
	)
	fs.Var(&tags, "tag", "全対象に共通のタグ。繰り返し指定する")
	fs.Var(&aliases, "alias", "全対象に共通の別名。繰り返し指定する")
	fs.Var(&exclude, "exclude", "除外するファイルやディレクトリ(gitignore 風)。繰り返し指定できる")
	fs.Usage = func() {
		fmt.Fprint(stderr, "使い方: okf-stamp apply [PATH...] [フラグ]\n\nフラグ:\n")
		fs.PrintDefaults()
	}

	paths, err := parseInterspersed(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if *format != "table" && *format != "json" {
		fmt.Fprintf(stderr, "--format は table または json を指定する: %q\n", *format)
		return exitUsage
	}
	defMode, err := stamp.ParseMode(*mode)
	if err != nil {
		fmt.Fprintf(stderr, "--%v\n", err)
		return exitUsage
	}

	flags := buildFlags(*typ, *status, tags, aliases)

	// 境界は、実行時のカレントディレクトリ。
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}

	var m *stamp.Manifest
	if *manifest != "" {
		if m, err = loadManifest(*manifest, stdin); err != nil {
			fmt.Fprintln(stderr, err)
			return exitUsage
		}
	}

	rep, err := stamp.Run(stamp.Options{
		Root: root, Paths: paths, NoRecurse: *noRecurse, Exclude: exclude,
		Manifest: m, Flags: flags, DefaultMode: defMode, Write: *write,
	})
	var pathErr *stamp.PathError
	switch {
	case errors.Is(err, stamp.ErrNoTargets), errors.As(err, &pathErr):
		fmt.Fprintln(stderr, err)
		return exitUsage
	case err != nil:
		fmt.Fprintln(stderr, err)
		return exitError
	}

	if *format == "json" {
		err = writeJSON(stdout, rep)
	} else {
		err = writeTable(stdout, rep)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}
	writeSummary(stderr, rep, *verbose)

	switch {
	case rep.HasWriteFailure():
		return exitError
	case rep.HasSkip():
		return exitPartial
	}
	return exitOK
}

// buildFlags は専用フラグの値を、全対象に共通の frontmatter にする。
func buildFlags(typ, status string, tags, aliases []string) map[string]any {
	out := map[string]any{}
	if typ != "" {
		out["type"] = typ
	}
	if status != "" {
		out["status"] = status
	}
	if len(tags) > 0 {
		out["tags"] = toAny(tags)
	}
	if len(aliases) > 0 {
		out["aliases"] = toAny(aliases)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func toAny(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

func loadManifest(name string, stdin io.Reader) (*stamp.Manifest, error) {
	r := stdin
	if name != "-" {
		f, err := os.Open(name)
		if err != nil {
			return nil, fmt.Errorf("マニフェストを開けない: %w", err)
		}
		defer f.Close()
		r = f
	}
	return stamp.ParseManifest(r)
}
