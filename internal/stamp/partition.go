package stamp

import (
	"path"
	"sort"
	"strings"
)

// DefaultMaxFiles は partition の 1 グループの既定の上限(設計書 5.5 節)。
const DefaultMaxFiles = 100

// Group は partition が作る、互いに重ならない処理単位。
type Group struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
	Files     int    `json:"files"`
	Oversize  bool   `json:"oversize,omitempty"`
}

// PartitionResult は partition の結果。
type PartitionResult struct {
	Total  int     `json:"total"`
	Groups []Group `json:"groups"`
}

// Partition は files を、フォルダ単位で互いに重ならないグループに分ける。
// scopes は PATH のディレクトリ。files は ResolveTargets の結果。
// scopes の配下にないファイル(PATH で直接指定されたファイル)は、1 ファイルのグループにする。
//
// 配下のファイル数が max 以下のフォルダは、再帰的な 1 グループにする。
// 超えるフォルダは、サブフォルダごとに分け、直下のファイルは recursive: false の
// グループにする。直下だけで max を超えるときは oversize を付ける。
func Partition(files []string, scopes []string, max int) PartitionResult {
	if max <= 0 {
		max = DefaultMaxFiles
	}
	direct := map[string]int{} // フォルダ → 直下のファイル数
	for _, f := range files {
		direct[dirOf(f)]++
	}
	children := map[string][]string{}
	seen := map[string]bool{}
	var link func(d string)
	link = func(d string) {
		if d == "." || seen[d] {
			return
		}
		seen[d] = true
		parent := dirOf(d)
		children[parent] = append(children[parent], d)
		link(parent)
	}
	for d := range direct {
		link(d)
	}

	var total func(d string) int
	memo := map[string]int{}
	total = func(d string) int {
		if n, ok := memo[d]; ok {
			return n
		}
		n := direct[d]
		for _, c := range children[d] {
			n += total(c)
		}
		memo[d] = n
		return n
	}

	res := PartitionResult{Groups: []Group{}}
	var walk func(d string)
	walk = func(d string) {
		n := total(d)
		if n == 0 {
			return
		}
		if n <= max {
			res.Groups = append(res.Groups, Group{Path: d, Recursive: true, Files: n})
			return
		}
		if direct[d] > 0 {
			res.Groups = append(res.Groups, Group{Path: d, Recursive: false, Files: direct[d], Oversize: direct[d] > max})
		}
		for _, c := range children[d] {
			walk(c)
		}
	}

	roots := uniqueDirs(scopes)
	for _, d := range roots {
		walk(d)
	}
	for _, f := range files {
		if !underAny(roots, f) {
			res.Groups = append(res.Groups, Group{Path: f, Recursive: false, Files: 1})
		}
	}
	sort.SliceStable(res.Groups, func(i, j int) bool { return res.Groups[i].Path < res.Groups[j].Path })
	res.Total = len(files)
	return res
}

func dirOf(f string) string { return path.Dir(f) }

// uniqueDirs は重複と、別の PATH の配下にある PATH を除く。
func uniqueDirs(ds []string) []string {
	sorted := append([]string(nil), ds...)
	sort.Strings(sorted)
	var out []string
	for _, d := range sorted {
		if len(out) > 0 {
			last := out[len(out)-1]
			if d == last || last == "." || strings.HasPrefix(d, last+"/") {
				continue
			}
		}
		out = append(out, d)
	}
	return out
}

// underAny は f が、dirs のどれかの配下にあるかを返す。
func underAny(dirs []string, f string) bool {
	for _, d := range dirs {
		if d == "." || strings.HasPrefix(f, d+"/") {
			return true
		}
	}
	return false
}
