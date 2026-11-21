package stamp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// Mode は既存の値の扱い。
type Mode string

const (
	// ModeFill は、無いキーだけを補う(既定)。
	ModeFill Mode = "fill"
	// ModeOverwrite は、入力のキーで置き換える。
	ModeOverwrite Mode = "overwrite"
	// ModeMerge は、tags と aliases を和集合にする。ほかの既存キーは残す。
	ModeMerge Mode = "merge"
)

// ParseMode は文字列を Mode にする。空文字は ModeFill。
func ParseMode(s string) (Mode, error) {
	switch Mode(s) {
	case "":
		return ModeFill, nil
	case ModeFill, ModeOverwrite, ModeMerge:
		return Mode(s), nil
	}
	return "", fmt.Errorf("mode は fill、overwrite、merge のいずれか: %q", s)
}

// ManifestVersion は、扱えるマニフェストの版。
const ManifestVersion = 1

// Manifest はエージェントが渡す入力。
type Manifest struct {
	Version int     `json:"version"`
	Mode    string  `json:"mode,omitempty"`
	Files   []Entry `json:"files"`
}

// Entry は1ファイル分の入力。
type Entry struct {
	Path        string         `json:"path"`
	Mode        string         `json:"mode,omitempty"`
	Frontmatter map[string]any `json:"frontmatter"`
}

// EntryMode は Entry の実効モードを返す。files[].mode、全体の mode、def の順に採る。
func (m *Manifest) EntryMode(e Entry, def Mode) Mode {
	for _, s := range []string{e.Mode, m.Mode} {
		if s != "" {
			return Mode(s)
		}
	}
	return def
}

// ParseManifest はマニフェストを読み、書式を検証する。
// エラーは書式違いで、呼び出し側は引数エラー(終了コード 2)として扱う。
// frontmatter の中身の検証は Validate が行う。
func ParseManifest(r io.Reader) (*Manifest, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("マニフェストを読めない: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	dec.UseNumber()

	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("マニフェストの書式が不正: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("マニフェストの末尾に余分なデータがある")
	}

	if m.Version != ManifestVersion {
		return nil, fmt.Errorf("version は %d にする: %d", ManifestVersion, m.Version)
	}
	if m.Mode != "" {
		if _, err := ParseMode(m.Mode); err != nil {
			return nil, err
		}
	}

	seen := map[string]int{}
	for i := range m.Files {
		e := &m.Files[i]
		if strings.TrimSpace(e.Path) == "" {
			return nil, fmt.Errorf("files[%d].path がない", i)
		}
		if e.Mode != "" {
			if _, err := ParseMode(e.Mode); err != nil {
				return nil, fmt.Errorf("files[%d]: %w", i, err)
			}
		}
		if e.Frontmatter == nil {
			return nil, fmt.Errorf("files[%d].frontmatter がない(%s)", i, e.Path)
		}
		key := fold(NormalizePath(e.Path))
		if j, dup := seen[key]; dup {
			return nil, fmt.Errorf("path が重複している: files[%d] と files[%d](%s)", j, i, e.Path)
		}
		seen[key] = i
	}
	return &m, nil
}

// NormalizePath は "\" を "/" にし、NFC にして、パスを整える。
func NormalizePath(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	return path.Clean(norm.NFC.String(p))
}
