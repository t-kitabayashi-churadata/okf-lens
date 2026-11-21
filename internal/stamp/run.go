package stamp

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/t-kitabayashi-churadata/okf-lens/internal/source"
)

// ErrNoTargets は PATH もマニフェストも指定されていないときに返る。
// 呼び出し側は引数エラー(終了コード 2)として扱う。
var ErrNoTargets = errors.New("対象がない。PATH か --manifest を指定する(カレント全体を対象にするときは . を指定する)")

// PathError は PATH 引数が、境界の外を指す、または存在しないときに返る。
// 何も処理せずに止める。呼び出し側は引数エラー(終了コード 2)として扱う。
type PathError struct {
	Path string
	Msg  string
}

func (e *PathError) Error() string { return fmt.Sprintf("PATH %q: %s", e.Path, e.Msg) }

// Options は Run の設定。
type Options struct {
	// Root は境界。この中のファイルだけを読み書きする。CLI はカレントディレクトリを渡す。
	// マニフェストと PATH のパスの基準で、.okf-lensignore を探す場所でもある。
	Root string
	// Paths は PATH 引数。Root からの相対パス(Root の中を指す絶対パスも可)。
	Paths []string
	// NoRecurse が true のとき、ディレクトリの直下のファイルだけを対象にする。
	NoRecurse bool
	// Exclude は除外パターン(--exclude)。
	Exclude []string
	// Manifest はファイルごとの入力。nil なら PATH のファイルにフラグの値だけを適用する。
	Manifest *Manifest
	// Flags は全対象に共通の値(--type、--tag など)。マニフェストの同じキーが優先する。
	Flags map[string]any
	// DefaultMode はマニフェストに mode がないときのモード。空なら fill。
	DefaultMode Mode
	// Write が true のときだけ、ファイルに書き込む。
	Write bool
}

// Scope は実行の担当範囲。並列実行の結果を集約するために、レポートに含める。
type Scope struct {
	Paths     []string `json:"paths"`
	Recursive bool     `json:"recursive"`
}

// FileResult は1ファイルの結果。
type FileResult struct {
	Path        string   `json:"path"`
	Action      string   `json:"action"` // add、update、unchanged、skip
	Added       []string `json:"-"`      // changes に含まれる。table の DETAIL に使う
	Overwritten []string `json:"-"`      // 同上
	Kept        []string `json:"kept"`
	Errors      []*Error `json:"errors"`
	Warnings    []*Error `json:"warnings"`
	// Changes は書いた(dry-run では書く)キーの、変更前と変更後の値。
	Changes []KeyChange `json:"changes"`
}

// ActionSkip は入力エラーなどで、そのファイルを変更しなかったときの Action。
const ActionSkip = "skip"

// Report は Run の結果。
type Report struct {
	DryRun  bool           `json:"dry_run"`
	Scope   Scope          `json:"scope"`
	Summary map[string]int `json:"summary"`
	Files   []FileResult   `json:"files"`
}

// HasSkip は入力エラーでスキップしたファイルがあるかを返す(終了コード 3)。
func (r *Report) HasSkip() bool { return r.Summary[ActionSkip] > 0 }

// HasWriteFailure は書き込みに失敗したファイルがあるかを返す(終了コード 1)。
func (r *Report) HasWriteFailure() bool {
	for _, f := range r.Files {
		for _, e := range f.Errors {
			if e.Code == CodeWriteFailed {
				return true
			}
		}
	}
	return false
}

type scopeItem struct {
	path  string // ルートからの相対。"." はルート
	isDir bool
}

// Run は対象を決め、検証し、Write が true なら書き込む。
// ファイルごとの入力エラーは Report に記録し、エラーとしては返さない。
func Run(opts Options) (*Report, error) {
	if len(opts.Paths) == 0 && opts.Manifest == nil {
		return nil, ErrNoTargets
	}
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stamp: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("stamp: %s はディレクトリではない", opts.Root)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("stamp: %w", err)
	}

	r := &runner{
		opts:     opts,
		root:     root,
		realRoot: realRoot,
		targets:  map[string]*Entry{},
		skips:    map[string][]*Error{},
	}
	if err := r.collect(); err != nil {
		return nil, err
	}
	return r.process(), nil
}

type runner struct {
	opts     Options
	root     string
	realRoot string
	scopes   []scopeItem
	targets  map[string]*Entry // パス → 入力(PATH だけの対象は nil)
	skips    map[string][]*Error
}

func (r *runner) skip(p string, e *Error) { r.skips[p] = append(r.skips[p], e) }

// relPath は渡されたパスを、境界からの相対パス("/" 区切り)にする。
// 境界の外を指すときは ok が false。
func (r *runner) relPath(p string) (rel string, ok bool) {
	if filepath.IsAbs(p) {
		var err error
		if p, err = filepath.Rel(r.root, p); err != nil {
			return "", false
		}
	}
	rel = NormalizePath(filepath.ToSlash(p))
	if rel == ".." || strings.HasPrefix(rel, "../") || path.IsAbs(rel) {
		return "", false
	}
	return rel, true
}

// collect は PATH とマニフェストから、対象ファイルを集める(5.2 節)。
// PATH が境界の外、または存在しないときは、何も処理せずに *PathError を返す。
func (r *runner) collect() error {
	for _, p := range r.opts.Paths {
		rel, ok := r.relPath(p)
		if !ok {
			return &PathError{Path: p, Msg: "境界(カレントディレクトリ)の外を指している"}
		}
		fi, err := os.Stat(filepath.Join(r.root, filepath.FromSlash(rel)))
		if err != nil {
			return &PathError{Path: p, Msg: "存在しない"}
		}
		r.scopes = append(r.scopes, scopeItem{path: rel, isDir: fi.IsDir()})
	}

	for _, sc := range r.scopes {
		if !sc.isDir {
			r.addExplicit(sc.path, nil)
			continue
		}
		files, err := source.List(os.DirFS(r.root), source.ListOptions{
			Exclude: r.opts.Exclude, Roots: []string{sc.path}, NoRecurse: r.opts.NoRecurse,
		})
		if err != nil {
			return err
		}
		for _, f := range files {
			if _, dup := r.targets[f]; !dup {
				r.targets[f] = nil
			}
		}
	}

	if r.opts.Manifest != nil {
		for i := range r.opts.Manifest.Files {
			e := &r.opts.Manifest.Files[i]
			rel, ok := r.relPath(e.Path)
			if !ok {
				r.skip(e.Path, errf(CodeOutsideRoot, "境界(カレントディレクトリ)の外を指している"))
				continue
			}
			if len(r.opts.Paths) > 0 && !r.inScope(rel) {
				r.skip(rel, errf(CodeOutsideScope, "PATH 指定の範囲の外を指している"))
				continue
			}
			r.addExplicit(rel, e)
		}
	}
	return nil
}

// inScope は rel が PATH 指定の範囲に入るかを返す。
func (r *runner) inScope(rel string) bool {
	for _, s := range r.scopes {
		switch {
		case !s.isDir:
			if rel == s.path {
				return true
			}
		case s.path == ".":
			if !r.opts.NoRecurse || !strings.Contains(rel, "/") {
				return true
			}
		case strings.HasPrefix(rel, s.path+"/"):
			if !r.opts.NoRecurse || !strings.Contains(strings.TrimPrefix(rel, s.path+"/"), "/") {
				return true
			}
		}
	}
	return false
}

// addExplicit は、直接指定されたファイル(PATH のファイル、マニフェストのパス)を検査して加える。
// .okf-lensignore と --exclude は適用しない。予約ファイルは適用しない。
func (r *runner) addExplicit(rel string, e *Entry) {
	base := path.Base(rel)
	switch {
	case source.IsReserved(base):
		r.skip(rel, errf(CodeReservedName, "予約ファイル(index.md、log.md)には frontmatter を付けない"))
		return
	case !strings.EqualFold(path.Ext(base), ".md"):
		r.skip(rel, errf(CodeNotMarkdown, ".md ファイルではない"))
		return
	}
	fi, err := os.Stat(filepath.Join(r.root, filepath.FromSlash(rel)))
	if err != nil {
		r.skip(rel, errf(CodeNotFound, "ファイルがない"))
		return
	}
	if fi.IsDir() {
		r.skip(rel, errf(CodeNotMarkdown, "ディレクトリは個別のファイルとして指定できない"))
		return
	}
	// 同じファイルが PATH とマニフェストの両方にあるときは、マニフェストの入力を使う。
	if _, exists := r.targets[rel]; !exists || e != nil {
		r.targets[rel] = e
	}
}

// process は集めた対象を、パスの辞書順に処理する。
func (r *runner) process() *Report {
	paths := map[string]bool{}
	for p := range r.targets {
		paths[p] = true
	}
	for p := range r.skips {
		paths[p] = true
	}
	sorted := make([]string, 0, len(paths))
	for p := range paths {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)

	rep := &Report{
		DryRun:  !r.opts.Write,
		Summary: map[string]int{"add": 0, "update": 0, "unchanged": 0, ActionSkip: 0},
		Files:   []FileResult{},
		Scope:   r.scope(),
	}
	for _, p := range sorted {
		var fr FileResult
		if errs, bad := r.skips[p]; bad {
			fr = newResult(p)
			fr.Action = ActionSkip
			fr.Errors = errs
		} else {
			fr = r.processFile(p, r.targets[p])
		}
		rep.Summary[fr.Action]++
		rep.Files = append(rep.Files, fr)
	}
	return rep
}

func (r *runner) scope() Scope {
	s := Scope{Paths: []string{}, Recursive: !r.opts.NoRecurse}
	for _, it := range r.scopes {
		s.Paths = append(s.Paths, it.path)
	}
	return s
}

func newResult(p string) FileResult {
	return FileResult{
		Path: p, Added: []string{}, Overwritten: []string{}, Kept: []string{},
		Errors: []*Error{}, Warnings: []*Error{}, Changes: []KeyChange{},
	}
}

func (r *runner) processFile(rel string, entry *Entry) FileResult {
	fr := newResult(rel)
	fr.Action = ActionSkip

	abs := filepath.Join(r.root, filepath.FromSlash(rel))
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		fr.Errors = append(fr.Errors, errf(CodeUnreadable, err.Error()))
		return fr
	}
	if !within(r.realRoot, real) {
		fr.Errors = append(fr.Errors, errf(CodeOutsideRoot, "シンボリックリンクの先が境界(カレントディレクトリ)の外にある"))
		return fr
	}
	data, err := os.ReadFile(real)
	if err != nil {
		fr.Errors = append(fr.Errors, errf(CodeUnreadable, err.Error()))
		return fr
	}

	// マニフェストの値がフラグより優先する(キー単位)。
	raw := make(map[string]any, len(r.opts.Flags))
	for k, v := range r.opts.Flags {
		raw[k] = v
	}
	mode := r.opts.DefaultMode
	if mode == "" {
		mode = ModeFill
	}
	if entry != nil {
		for k, v := range entry.Frontmatter {
			raw[k] = v
		}
		mode = r.opts.Manifest.EntryMode(*entry, mode)
	}

	input, errs := Validate(raw)
	if errs != nil {
		fr.Errors = errs
		return fr
	}
	fr.Warnings = append(fr.Warnings, Warnings(input)...)

	ch, errs := Apply(rel, data, input, mode)
	if errs != nil {
		fr.Errors = errs
		return fr
	}

	fr.Action = string(ch.Action)
	fr.Added = nonNilStrings(ch.Added)
	fr.Overwritten = nonNilStrings(ch.Overwritten)
	fr.Kept = nonNilStrings(ch.Kept)
	if ch.Changes != nil {
		fr.Changes = ch.Changes
	}
	if ch.New == nil {
		return fr
	}
	if r.opts.Write {
		if e := writeAtomic(real, data, ch.New); e != nil {
			fr.Action = ActionSkip
			fr.Added, fr.Overwritten, fr.Kept = []string{}, []string{}, []string{}
			fr.Changes = []KeyChange{}
			fr.Errors = append(fr.Errors, e)
		}
	}
	return fr
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// within は p が dir の中(dir 自身を含む)にあるかを返す。
func within(dir, p string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// writeAtomic は同じディレクトリの一時ファイルに書いて、rename で置き換える。
// 書き込み直前に内容を読み直し、計画時(orig)から変わっていたら書かない。
func writeAtomic(target string, orig, content []byte) *Error {
	cur, err := os.ReadFile(target)
	if err != nil {
		return errf(CodeWriteFailed, err.Error())
	}
	if !bytes.Equal(cur, orig) {
		return errf(CodeChangedDuringRun, "計画後にファイルが変更された。書き込まなかった")
	}
	info, err := os.Stat(target)
	if err != nil {
		return errf(CodeWriteFailed, err.Error())
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".okf-stamp-*")
	if err != nil {
		return errf(CodeWriteFailed, err.Error())
	}
	tmpName := tmp.Name()
	cleanup := func(e error) *Error {
		tmp.Close()
		os.Remove(tmpName)
		return errf(CodeWriteFailed, e.Error())
	}
	if _, err := tmp.Write(content); err != nil {
		return cleanup(err)
	}
	if err := tmp.Chmod(info.Mode().Perm()); err != nil && !errors.Is(err, fs.ErrPermission) {
		return cleanup(err)
	}
	if err := tmp.Close(); err != nil {
		return cleanup(err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		os.Remove(tmpName)
		return errf(CodeWriteFailed, err.Error())
	}
	return nil
}
