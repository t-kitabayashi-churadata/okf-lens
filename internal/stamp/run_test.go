package stamp

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// tree はテスト用のディレクトリを作り、ルートのパスを返す。
func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func read(t *testing.T, root, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func byPath(rep *Report) map[string]FileResult {
	m := map[string]FileResult{}
	for _, f := range rep.Files {
		m[f.Path] = f
	}
	return m
}

func firstCode(f FileResult) string {
	if len(f.Errors) == 0 {
		return ""
	}
	return f.Errors[0].Code
}

func TestRun_noTargets(t *testing.T) {
	if _, err := Run(Options{Root: t.TempDir(), Flags: map[string]any{"type": "API"}}); !errors.Is(err, ErrNoTargets) {
		t.Errorf("err = %v, want ErrNoTargets", err)
	}
}

func TestRun_dirWithFlags_dryRunThenWrite(t *testing.T) {
	root := tree(t, map[string]string{
		"a.md":           "本文A\n",
		"sub/b.md":       "---\ntype: 画面\n---\n本文B\n",
		"index.md":       "---\nokf_version: \"0.2\"\n---\n",
		"sub/log.md":     "履歴",
		"note.txt":       "x",
		".git/hidden.md": "x",
	})
	opts := Options{Root: root, Paths: []string{"."}, Flags: map[string]any{
		"type": "API", "tags": []any{"設計"},
	}}

	rep, err := Run(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.DryRun || len(rep.Files) != 2 {
		t.Fatalf("dry-run のはず、対象は2件のはず: %+v", rep)
	}
	if read(t, root, "a.md") != "本文A\n" {
		t.Error("dry-run でファイルが変わった")
	}
	got := byPath(rep)
	if got["a.md"].Action != "add" || got["sub/b.md"].Action != "update" {
		t.Errorf("actions: %+v", rep.Files)
	}
	if strings.Join(got["sub/b.md"].Kept, ",") != "type" {
		t.Errorf("kept = %v", got["sub/b.md"].Kept)
	}
	if rep.Summary["add"] != 1 || rep.Summary["update"] != 1 || rep.Summary["skip"] != 0 {
		t.Errorf("summary = %v", rep.Summary)
	}

	opts.Write = true
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	if want := "---\ntype: API\ntags:\n  - 設計\n---\n\n本文A\n"; read(t, root, "a.md") != want {
		t.Errorf("a.md = %q", read(t, root, "a.md"))
	}
	if want := "---\ntype: 画面\ntags:\n  - 設計\n---\n本文B\n"; read(t, root, "sub/b.md") != want {
		t.Errorf("sub/b.md = %q", read(t, root, "sub/b.md"))
	}
	if read(t, root, "index.md") != "---\nokf_version: \"0.2\"\n---\n" {
		t.Error("予約ファイルが変わった")
	}

	// 2回目はすべて unchanged。一時ファイルも残らない。
	rep, _ = Run(opts)
	if rep.Summary["unchanged"] != 2 {
		t.Errorf("2回目 summary = %v", rep.Summary)
	}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".okf-stamp-") {
			t.Errorf("一時ファイルが残った: %s", e.Name())
		}
	}
}

func TestRun_missingTypeIsSkipped(t *testing.T) {
	root := tree(t, map[string]string{"a.md": "本文\n", "b.md": "本文\n"})
	m := &Manifest{Version: 1, Files: []Entry{
		{Path: "a.md", Frontmatter: map[string]any{"type": "API"}},
	}}
	rep, err := Run(Options{Root: root, Paths: []string{"."}, Manifest: m, Write: true})
	if err != nil {
		t.Fatal(err)
	}
	got := byPath(rep)
	if got["a.md"].Action != "add" || got["b.md"].Action != "skip" || firstCode(got["b.md"]) != CodeMissingType {
		t.Errorf("results: %+v", rep.Files)
	}
	if !rep.HasSkip() {
		t.Error("HasSkip が false")
	}
	if read(t, root, "b.md") != "本文\n" {
		t.Error("スキップしたファイルが変わった")
	}
}

func TestRun_manifestBeatsFlags(t *testing.T) {
	root := tree(t, map[string]string{"a.md": "本文\n", "b.md": "本文\n"})
	m := &Manifest{Version: 1, Files: []Entry{
		{Path: "a.md", Frontmatter: map[string]any{"type": "画面"}},
	}}
	_, err := Run(Options{Root: root, Paths: []string{"."}, Manifest: m, Write: true,
		Flags: map[string]any{"type": "API", "tags": []any{"x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(t, root, "a.md"), "type: 画面") || !strings.Contains(read(t, root, "a.md"), "- x") {
		t.Errorf("a.md = %q", read(t, root, "a.md"))
	}
	if !strings.Contains(read(t, root, "b.md"), "type: API") {
		t.Errorf("b.md = %q", read(t, root, "b.md"))
	}
}

func TestRun_pathProblems(t *testing.T) {
	root := tree(t, map[string]string{
		"a.md": "本文\n", "index.md": "目次", "memo.txt": "x", "dir/x.md": "本文\n",
	})
	fm := map[string]any{"type": "API"}
	m := &Manifest{Version: 1, Files: []Entry{
		{Path: "index.md", Frontmatter: fm},
		{Path: "memo.txt", Frontmatter: fm},
		{Path: "nothing.md", Frontmatter: fm},
		{Path: "../x.md", Frontmatter: fm},
		{Path: "dir", Frontmatter: fm},
	}}
	rep, err := Run(Options{Root: root, Manifest: m})
	if err != nil {
		t.Fatal(err)
	}
	got := byPath(rep)
	want := map[string]string{
		"index.md": CodeReservedName, "memo.txt": CodeNotMarkdown, "nothing.md": CodeNotFound,
		"../x.md": CodeOutsideRoot, "dir": CodeNotMarkdown,
	}
	for p, code := range want {
		if firstCode(got[p]) != code {
			t.Errorf("%s: code = %q, want %q", p, firstCode(got[p]), code)
		}
	}
	if len(rep.Files) != len(want) || rep.Summary["skip"] != len(want) {
		t.Errorf("files = %d, summary = %v", len(rep.Files), rep.Summary)
	}
}

func TestRun_outsideScope(t *testing.T) {
	root := tree(t, map[string]string{"api/a.md": "本文\n", "screen/b.md": "本文\n", "api/sub/c.md": "本文\n"})
	fm := map[string]any{"type": "API"}
	m := &Manifest{Version: 1, Files: []Entry{
		{Path: "api/a.md", Frontmatter: fm},
		{Path: "screen/b.md", Frontmatter: fm}, // 別の担当のフォルダ
		{Path: "api/sub/c.md", Frontmatter: fm},
	}}
	rep, err := Run(Options{Root: root, Paths: []string{"api"}, NoRecurse: true, Manifest: m, Write: true})
	if err != nil {
		t.Fatal(err)
	}
	got := byPath(rep)
	if got["api/a.md"].Action != "add" {
		t.Errorf("api/a.md: %+v", got["api/a.md"])
	}
	for _, p := range []string{"screen/b.md", "api/sub/c.md"} {
		if firstCode(got[p]) != CodeOutsideScope {
			t.Errorf("%s: code = %q, want outside_scope", p, firstCode(got[p]))
		}
		if read(t, root, p) != "本文\n" {
			t.Errorf("%s が書き換わった", p)
		}
	}
	if rep.Scope.Recursive || len(rep.Scope.Paths) != 1 || rep.Scope.Paths[0] != "api" {
		t.Errorf("scope = %+v", rep.Scope)
	}
}

func TestRun_noRecurse(t *testing.T) {
	root := tree(t, map[string]string{"a.md": "x", "sub/b.md": "x"})
	rep, err := Run(Options{Root: root, Paths: []string{"."}, NoRecurse: true,
		Flags: map[string]any{"type": "API"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Files) != 1 || rep.Files[0].Path != "a.md" {
		t.Errorf("files = %+v", rep.Files)
	}
}

func TestRun_explicitFileIgnoresExclude(t *testing.T) {
	root := tree(t, map[string]string{"drafts/wip.md": "x", ".okf-lensignore": "drafts/\n"})
	flags := map[string]any{"type": "API"}
	rep, _ := Run(Options{Root: root, Paths: []string{"."}, Flags: flags})
	if len(rep.Files) != 0 {
		t.Errorf("ディレクトリ指定では除外される: %+v", rep.Files)
	}
	rep, _ = Run(Options{Root: root, Paths: []string{"drafts/wip.md"}, Flags: flags})
	if len(rep.Files) != 1 || rep.Files[0].Action != "add" {
		t.Errorf("直接指定では除外されない: %+v", rep.Files)
	}
}

func TestRun_warningsAndChanges(t *testing.T) {
	root := tree(t, map[string]string{"a.md": "---\ntype: API\n---\n本文\n"})
	rep, err := Run(Options{Root: root, Paths: []string{"a.md"},
		Flags: map[string]any{"tag": "x"}})
	if err != nil {
		t.Fatal(err)
	}
	f := rep.Files[0]
	if f.Action != "update" || len(f.Warnings) != 1 || f.Warnings[0].Code != CodeLegacyKey {
		t.Errorf("result = %+v", f)
	}
	if len(f.Changes) != 1 || f.Changes[0].Key != "tag" || f.Changes[0].Before != nil || f.Changes[0].After != "x" {
		t.Errorf("changes = %+v", f.Changes)
	}
	if rep.HasSkip() {
		t.Error("警告だけなら skip にしない")
	}
}

func TestRun_invalidInputSkipsOnlyThatFile(t *testing.T) {
	root := tree(t, map[string]string{"a.md": "x", "b.md": "x"})
	m := &Manifest{Version: 1, Files: []Entry{
		{Path: "a.md", Frontmatter: map[string]any{"type": "API", "tags": []any{"2026"}}},
		{Path: "b.md", Frontmatter: map[string]any{"type": "API"}},
	}}
	rep, _ := Run(Options{Root: root, Manifest: m, Write: true})
	got := byPath(rep)
	if firstCode(got["a.md"]) != CodeInvalidTag || got["b.md"].Action != "add" {
		t.Errorf("results = %+v", rep.Files)
	}
	if read(t, root, "a.md") != "x" {
		t.Error("不正な入力のファイルが変わった")
	}
}

func TestRun_symlinkOutsideRoot(t *testing.T) {
	outside := tree(t, map[string]string{"secret.md": "外のファイル\n"})
	root := tree(t, map[string]string{"a.md": "x"})
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(root, "link.md")); err != nil {
		t.Skipf("シンボリックリンクを作れない: %v", err)
	}
	rep, err := Run(Options{Root: root, Paths: []string{"link.md"}, Write: true,
		Flags: map[string]any{"type": "API"}})
	if err != nil {
		t.Fatal(err)
	}
	if firstCode(rep.Files[0]) != CodeOutsideRoot {
		t.Errorf("result = %+v", rep.Files[0])
	}
	if read(t, outside, "secret.md") != "外のファイル\n" {
		t.Error("ルートの外のファイルが書き換わった")
	}
}

func TestRun_preservesPermissionAndSymlinkInside(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("パーミッションの検査は Windows では行わない")
	}
	root := tree(t, map[string]string{"real/a.md": "x"})
	p := filepath.Join(root, "real", "a.md")
	if err := os.Chmod(p, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(p, filepath.Join(root, "link.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Options{Root: root, Paths: []string{"link.md"}, Write: true,
		Flags: map[string]any{"type": "API"}}); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Lstat(filepath.Join(root, "link.md"))
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("シンボリックリンクが通常ファイルに置き換わった")
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o640 {
		t.Errorf("パーミッション = %v", st.Mode().Perm())
	}
	if !strings.HasPrefix(read(t, root, "real/a.md"), "---\ntype: API\n") {
		t.Errorf("リンク先に書かれていない: %q", read(t, root, "real/a.md"))
	}
}

func TestWriteAtomic_detectsChange(t *testing.T) {
	root := tree(t, map[string]string{"a.md": "変更後の内容\n"})
	p := filepath.Join(root, "a.md")
	e := writeAtomic(p, []byte("計画時の内容\n"), []byte("新しい内容\n"))
	if e == nil || e.Code != CodeChangedDuringRun {
		t.Fatalf("err = %v, want changed_during_run", e)
	}
	if read(t, root, "a.md") != "変更後の内容\n" {
		t.Error("競合したのに書き込んだ")
	}
}

func TestRun_pathErrors(t *testing.T) {
	root := tree(t, map[string]string{"a.md": "x", "sub/b.md": "x"})
	flags := map[string]any{"type": "API"}
	for name, paths := range map[string][]string{
		"境界の外":     {"../a.md"},
		"境界の外(絶対)": {filepath.Dir(root)},
		"存在しない":    {"nothing"},
		"正しいものと混在": {"sub", "nothing"},
	} {
		t.Run(name, func(t *testing.T) {
			var pe *PathError
			_, err := Run(Options{Root: root, Paths: paths, Flags: flags, Write: true})
			if !errors.As(err, &pe) {
				t.Fatalf("err = %v, want *PathError", err)
			}
			if read(t, root, "sub/b.md") != "x" {
				t.Error("PATH の誤りがあるのに、一部を書き込んだ")
			}
		})
	}

	// 境界の中を指す絶対パスは受け付ける。
	rep, err := Run(Options{Root: root, Paths: []string{filepath.Join(root, "sub")}, Flags: flags})
	if err != nil || len(rep.Files) != 1 || rep.Files[0].Path != "sub/b.md" {
		t.Errorf("rep = %+v, err = %v", rep, err)
	}
}

func TestRun_writeFailureIsReported(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("読み取り専用ディレクトリを使う検査は、Windows と root では行わない")
	}
	root := tree(t, map[string]string{"ro/a.md": "x", "ok/b.md": "x"})
	ro := filepath.Join(root, "ro")
	if err := os.Chmod(ro, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(ro, 0o755)
	rep, err := Run(Options{Root: root, Paths: []string{"."}, Write: true,
		Flags: map[string]any{"type": "API"}})
	if err != nil {
		t.Fatal(err)
	}
	got := byPath(rep)
	if firstCode(got["ro/a.md"]) != CodeWriteFailed || got["ok/b.md"].Action != "add" {
		t.Errorf("results = %+v", rep.Files)
	}
	if !rep.HasWriteFailure() {
		t.Error("HasWriteFailure が false")
	}
}
