package source

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
)

// ListOptions は List の設定。
type ListOptions struct {
	// Exclude は除外パターン(--exclude)。Scan と同じ書式と規則。
	Exclude []string
	// Roots は列挙するディレクトリ(fsys のルートからの相対パス)。
	// 空なら fsys 全体。互いに重なってもよく、結果は重複しない。
	Roots []string
	// NoRecurse が true のとき、各 Roots の直下のファイルだけを返す。
	NoRecurse bool
}

// ListDir は dir 配下の concept 候補を列挙する。
func ListDir(dir string, opts ListOptions) ([]string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("source: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("source: %s はディレクトリではない", dir)
	}
	return List(os.DirFS(dir), opts)
}

// List は concept になりうる Markdown ファイルのパスを、パースせずに列挙する。
// frontmatter がないファイルも含む。パスは fsys のルートからの相対で、
// 区切りは "/"、辞書順。
//
// Scan と同じ規則で、次のものを除く。
//   - "." で始まるディレクトリ
//   - 予約ファイル名(IsReserved)
//   - .md 以外のファイル
//   - 除外パターンに一致するもの。Roots の祖先が除外されているときは、配下も除く
//
// Roots が存在しない、またはディレクトリでないときはエラーを返す。
func List(fsys fs.FS, opts ListOptions) ([]string, error) {
	ex, err := loadMatcher(fsys, opts.Exclude)
	if err != nil {
		return nil, err
	}

	roots := opts.Roots
	if len(roots) == 0 {
		roots = []string{"."}
	}

	seen := map[string]bool{}
	var out []string

	for _, root := range roots {
		root = path.Clean(strings.ReplaceAll(root, `\`, "/"))
		if !fs.ValidPath(root) {
			return nil, fmt.Errorf("source: 不正なパス: %s", root)
		}
		info, err := fs.Stat(fsys, root)
		if err != nil {
			return nil, fmt.Errorf("source: %w", err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("source: %s はディレクトリではない", root)
		}
		if excludedByAncestor(ex, root) {
			continue
		}

		err = fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if p == root {
					return err
				}
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if p != root && ex.match(p, d.IsDir()) {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				if p == root {
					return nil
				}
				if opts.NoRecurse || strings.HasPrefix(d.Name(), ".") {
					return fs.SkipDir
				}
				return nil
			}
			if isConceptCandidate(d.Name()) && !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("source: %w", err)
		}
	}

	sort.Strings(out)
	return out, nil
}

// excludedByAncestor は root 自身と、その祖先ディレクトリが除外されているかを返す。
func excludedByAncestor(ex *matcher, root string) bool {
	if root == "." {
		return false
	}
	parts := strings.Split(root, "/")
	for i := range parts {
		if ex.match(strings.Join(parts[:i+1], "/"), true) {
			return true
		}
	}
	return false
}
