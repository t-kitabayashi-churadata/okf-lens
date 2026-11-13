package source

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// ErrInvalidPattern は除外パターンの書式が不正なときに返る。
var ErrInvalidPattern = errors.New("source: 除外パターンが不正")

// IgnoreFileName は検索ルートに置く除外設定ファイルの名前。
const IgnoreFileName = ".okf-lensignore"

// rule は1つの除外パターン。
//
// 書式は gitignore の簡易版:
//   - 空行と "#" で始まる行は無視する
//   - "/" を含まないパターンは、任意の階層の名前(ファイル名またはディレクトリ名)に一致する
//   - "/" を含むパターンは、検索ルートからの相対パス全体に一致する(先頭の "/" は無視する)
//   - 末尾の "/" はディレクトリだけに一致する
//   - "*" と "?" は "/" をまたがない。"**" だけの区切りは、0 個以上の階層に一致する
//   - 大文字小文字を区別しない。"\" は "/" として扱う
//   - 否定("!")は対応しない
type rule struct {
	segs     []string
	dirOnly  bool
	anyDepth bool
}

// matcher は除外パターンの集合。
type matcher struct{ rules []rule }

// fold は比較用に NFC 正規化と小文字化を行う。
func fold(s string) string { return strings.ToLower(norm.NFC.String(s)) }

// parseRule は1行のパターンを rule に変換する。skip が true なら、空行かコメント。
func parseRule(line string) (r rule, skip bool, err error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return rule{}, true, nil
	}
	if strings.HasPrefix(line, "!") {
		return rule{}, false, fmt.Errorf("%w: 否定パターン(!)には対応していない: %q", ErrInvalidPattern, line)
	}

	p := strings.ReplaceAll(fold(line), `\`, "/")
	if strings.HasSuffix(p, "/") {
		r.dirOnly = true
		p = strings.TrimRight(p, "/")
	}
	anchored := strings.Contains(p, "/")
	p = strings.TrimLeft(p, "/")
	if p == "" {
		return rule{}, false, fmt.Errorf("%w: %q", ErrInvalidPattern, line)
	}

	r.segs = strings.Split(p, "/")
	r.anyDepth = !anchored
	for _, seg := range r.segs {
		if seg == "" {
			return rule{}, false, fmt.Errorf("%w: 空の階層がある: %q", ErrInvalidPattern, line)
		}
		if _, err := path.Match(seg, ""); err != nil {
			return rule{}, false, fmt.Errorf("%w: %q: %v", ErrInvalidPattern, line, err)
		}
	}
	return r, false, nil
}

// newMatcher は、ファイル由来の行と、フラグ由来のパターンから matcher を作る。
// file は除外設定ファイルの内容(なければ nil)。
func newMatcher(file []byte, flags []string) (*matcher, error) {
	m := &matcher{}
	if file != nil {
		text := strings.TrimPrefix(string(file), "\xEF\xBB\xBF")
		for i, line := range strings.Split(text, "\n") {
			r, skip, err := parseRule(line)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %w", IgnoreFileName, i+1, err)
			}
			if !skip {
				m.rules = append(m.rules, r)
			}
		}
	}
	for _, f := range flags {
		r, skip, err := parseRule(f)
		if err != nil {
			return nil, fmt.Errorf("--exclude: %w", err)
		}
		if !skip {
			m.rules = append(m.rules, r)
		}
	}
	return m, nil
}

// match は、検索ルートからの相対パス(区切りは "/")が除外されるかを返す。
func (m *matcher) match(rel string, isDir bool) bool {
	if m == nil || len(m.rules) == 0 {
		return false
	}
	rel = fold(rel)
	segs := strings.Split(rel, "/")
	for _, r := range m.rules {
		if r.dirOnly && !isDir {
			continue
		}
		if r.anyDepth {
			if ok, _ := path.Match(r.segs[0], segs[len(segs)-1]); ok && len(r.segs) == 1 {
				return true
			}
			continue
		}
		if matchSegments(r.segs, segs) {
			return true
		}
	}
	return false
}

// matchSegments はパターンの各階層とパスの各階層を照合する。
// "**" は 0 個以上の階層に一致する。
func matchSegments(pat, segs []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			for len(pat) > 0 && pat[0] == "**" {
				pat = pat[1:]
			}
			if len(pat) == 0 {
				return true
			}
			for i := 0; i <= len(segs); i++ {
				if matchSegments(pat, segs[i:]) {
					return true
				}
			}
			return false
		}
		if len(segs) == 0 {
			return false
		}
		if ok, _ := path.Match(pat[0], segs[0]); !ok {
			return false
		}
		pat, segs = pat[1:], segs[1:]
	}
	return len(segs) == 0
}
