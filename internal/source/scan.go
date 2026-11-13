// Package source は検索ルート配下の concept ファイルを走査して読み込む。
package source

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"

	"github.com/t-kitabayashi-churadata/okf-lens/internal/okf"
)

// Skip は concept として読めなかったファイルと、その理由。
type Skip struct {
	Path string
	Err  error
}

// Result は走査の結果。
type Result struct {
	// Docs は読み込めた concept。パスの辞書順。
	Docs []*okf.Document
	// Skipped は読み飛ばした Markdown ファイル。
	// frontmatter や type がないファイルも含む。
	Skipped []Skip
}

// reservedNames は OKF が予約するファイル名。concept ではないため読まない。
var reservedNames = []string{"index.md", "log.md"}

// Options は走査の設定。
type Options struct {
	// Exclude は除外パターン(--exclude)。書式は ignore.go を参照。
	// 検索ルートの .okf-lensignore と合わせて適用する。
	Exclude []string
}

// ScanDir は dir 配下を走査する。
func ScanDir(dir string, opts Options) (*Result, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("source: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("source: %s はディレクトリではない", dir)
	}
	return Scan(os.DirFS(dir), opts)
}

// Scan は fsys の全 Markdown ファイルを読み、concept を集める。
//
// 読み飛ばすもの:
//   - "." で始まるディレクトリ(.git、.obsidian など)
//   - 予約ファイル名(index.md、log.md)
//   - 拡張子が .md 以外のファイル
//   - 除外パターンに一致するファイルとディレクトリ(読み飛ばしの記録にも残さない)
//
// 除外パターンは、fsys のルートにある .okf-lensignore と opts.Exclude から読む。
// パターンが不正なときは ErrInvalidPattern を返す。
//
// 個々のファイルの読み込みや解析の失敗では止まらず、Result.Skipped に記録する。
func Scan(fsys fs.FS, opts Options) (*Result, error) {
	ignoreFile, err := fs.ReadFile(fsys, IgnoreFileName)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("source: %s を読めない: %w", IgnoreFileName, err)
	}
	ex, err := newMatcher(ignoreFile, opts.Exclude)
	if err != nil {
		return nil, err
	}

	res := &Result{}

	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// ルートの読み取り失敗は致命的。配下の失敗は記録して続ける。
			if p == "." {
				return err
			}
			res.Skipped = append(res.Skipped, Skip{Path: p, Err: err})
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		if p != "." && ex.match(p, d.IsDir()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			if p != "." && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}

		if !isConceptCandidate(d.Name()) {
			return nil
		}

		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			res.Skipped = append(res.Skipped, Skip{Path: p, Err: err})
			return nil
		}

		doc, err := okf.Parse(p, data)
		if err != nil {
			res.Skipped = append(res.Skipped, Skip{Path: p, Err: err})
			return nil
		}
		res.Docs = append(res.Docs, doc)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("source: %w", err)
	}
	return res, nil
}

// isConceptCandidate は concept になりうるファイル名かを返す。
func isConceptCandidate(name string) bool {
	if !strings.EqualFold(path.Ext(name), ".md") {
		return false
	}
	for _, r := range reservedNames {
		if strings.EqualFold(name, r) {
			return false
		}
	}
	return true
}
