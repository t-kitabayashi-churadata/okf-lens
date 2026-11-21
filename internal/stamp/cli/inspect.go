package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/t-kitabayashi-churadata/okf-lens/internal/stamp"
)

// resolve は scan と partition に共通の、PATH からの対象の決定。
// 失敗したときは、表示済みで、終了コードを返す。
func resolve(paths []string, noRecurse bool, exclude []string, stderr io.Writer) (*stamp.Targets, string, int) {
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return nil, "", exitError
	}
	t, err := stamp.ResolveTargets(root, paths, noRecurse, exclude)
	var pathErr *stamp.PathError
	switch {
	case errors.Is(err, stamp.ErrNoTargets):
		fmt.Fprintln(stderr, "PATH を指定する")
		return nil, "", exitUsage
	case errors.As(err, &pathErr):
		fmt.Fprintln(stderr, err)
		return nil, "", exitUsage
	case err != nil:
		fmt.Fprintln(stderr, err)
		return nil, "", exitError
	}
	return t, root, exitOK
}

func runScan(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	noRecurse := fs.Bool("no-recurse", false, "ディレクトリの直下のファイルだけを対象にする")
	format := fs.String("format", "table", "出力形式(table または json)")
	var exclude multiFlag
	fs.Var(&exclude, "exclude", "除外するファイルやディレクトリ(gitignore 風)。繰り返し指定できる")
	fs.Usage = func() {
		fmt.Fprint(stderr, "使い方: okf-stamp scan [PATH...] [フラグ]\n\nフラグ:\n")
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
	t, root, code := resolve(paths, *noRecurse, exclude, stderr)
	if t == nil {
		return code
	}
	entries := stamp.Scan(root, t.Files)
	for _, s := range t.Skipped {
		fmt.Fprintf(stderr, "対象外: %s: %s\n", s.Path, s.Error.Message)
	}
	if *format == "json" {
		err = writeJSONValue(stdout, entries)
	} else {
		err = writeScanTable(stdout, entries)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}
	return exitOK
}

func writeScanTable(w io.Writer, entries []stamp.ScanEntry) error {
	if len(entries) == 0 {
		return nil
	}
	rows := [][]string{{"STATE", "PATH", "KEYS", "H1"}}
	for _, e := range entries {
		rows = append(rows, []string{e.State, cell(e.Path), cell(strings.Join(e.Keys, ",")), cell(e.H1)})
	}
	return writeRows(w, rows)
}

func runPartition(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("partition", flag.ContinueOnError)
	fs.SetOutput(stderr)
	maxFiles := fs.Int("max-files", stamp.DefaultMaxFiles, "1グループの最大ファイル数")
	format := fs.String("format", "table", "出力形式(table または json)")
	var exclude multiFlag
	fs.Var(&exclude, "exclude", "除外するファイルやディレクトリ(gitignore 風)。繰り返し指定できる")
	fs.Usage = func() {
		fmt.Fprint(stderr, "使い方: okf-stamp partition [PATH...] [フラグ]\n\nフラグ:\n")
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
	if *maxFiles < 1 {
		fmt.Fprintf(stderr, "--max-files は 1 以上を指定する: %d\n", *maxFiles)
		return exitUsage
	}
	t, root, code := resolve(paths, false, exclude, stderr)
	if t == nil {
		return code
	}
	var dirs []string
	for _, p := range paths {
		rel, isDir, err := stamp.DirPath(root, p)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return exitUsage
		}
		if !isDir {
			fmt.Fprintf(stderr, "PATH %q: partition はフォルダだけを指定できる\n", p)
			return exitUsage
		}
		dirs = append(dirs, rel)
	}
	res := stamp.Partition(t.Files, dirs, *maxFiles)
	if *format == "json" {
		err = writeJSONValue(stdout, res)
	} else {
		err = writePartitionTable(stdout, res)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}
	return exitOK
}

func writePartitionTable(w io.Writer, res stamp.PartitionResult) error {
	if len(res.Groups) == 0 {
		return nil
	}
	rows := [][]string{{"PATH", "RECURSIVE", "FILES", "NOTE"}}
	for _, g := range res.Groups {
		note := ""
		if g.Oversize {
			note = "oversize"
		}
		rows = append(rows, []string{cell(g.Path), strconv.FormatBool(g.Recursive), strconv.Itoa(g.Files), note})
	}
	return writeRows(w, rows)
}
