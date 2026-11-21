package stamp

import (
	"os"
	"path/filepath"
	"sort"
)

// Skipped は対象にできなかったファイル(予約ファイル、.md 以外など)。
type Skipped struct {
	Path  string
	Error *Error
}

// Targets は PATH から決めた対象ファイル。apply と同じ規則(設計書 5.2 節)で決める。
type Targets struct {
	// Files は境界からの相対パス("/" 区切り)。辞書順。
	Files []string
	// Skipped は PATH で直接指定されたが、対象にできなかったファイル。
	Skipped []Skipped
}

// ResolveTargets は paths の対象ファイルを決める。scan と partition が使う。
// PATH が境界の外、または存在しないときは *PathError を返す。
func ResolveTargets(root string, paths []string, noRecurse bool, exclude []string) (*Targets, error) {
	if len(paths) == 0 {
		return nil, ErrNoTargets
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	r := &runner{
		opts:    Options{Root: abs, Paths: paths, NoRecurse: noRecurse, Exclude: exclude},
		root:    abs,
		targets: map[string]*Entry{},
		skips:   map[string][]*Error{},
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, err
	}
	if err := r.collect(); err != nil {
		return nil, err
	}
	t := &Targets{}
	for p := range r.targets {
		t.Files = append(t.Files, p)
	}
	sort.Strings(t.Files)
	var skipped []string
	for p := range r.skips {
		skipped = append(skipped, p)
	}
	sort.Strings(skipped)
	for _, p := range skipped {
		for _, e := range r.skips[p] {
			t.Skipped = append(t.Skipped, Skipped{Path: p, Error: e})
		}
	}
	return t, nil
}

// DirPath は PATH を境界からの相対パスにし、ディレクトリかを返す。
// ResolveTargets が検証済みの PATH に使う。
func DirPath(root, p string) (rel string, isDir bool, err error) {
	r := &runner{root: root}
	rel, ok := r.relPath(p)
	if !ok {
		return "", false, &PathError{Path: p, Msg: "境界(カレントディレクトリ)の外を指している"}
	}
	fi, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return "", false, &PathError{Path: p, Msg: "存在しない"}
	}
	return rel, fi.IsDir(), nil
}
