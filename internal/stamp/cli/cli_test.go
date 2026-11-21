package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = Run(args, strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

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
	b, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestApply_dryRunTable(t *testing.T) {
	root := tree(t, map[string]string{"api/auth.md": "本文\n", "memo/idea.md": "本文\n"})
	t.Chdir(root)
	m := `{"version":1,"files":[{"path":"api/auth.md","frontmatter":{"type":"API","title":"認証API","tags":["設計"]}}]}`
	code, out, errOut := run(t, m, "apply", ".", "--manifest", "-")
	if code != 3 {
		t.Errorf("code = %d, want 3(type がないファイルがある)", code)
	}
	want := "ACTION  PATH          DETAIL\n" +
		"add     api/auth.md   +type +title +tags\n" +
		"skip    memo/idea.md  type がない\n"
	if out != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", out, want)
	}
	if !strings.Contains(errOut, "dry-run") || !strings.Contains(errOut, "2 件(add 1、update 0、unchanged 0、skip 1)") {
		t.Errorf("stderr = %q", errOut)
	}
	if read(t, root, "api/auth.md") != "本文\n" {
		t.Error("dry-run でファイルが変わった")
	}
}

func TestApply_writeWithFlags(t *testing.T) {
	root := tree(t, map[string]string{"a.md": "本文\n"})
	t.Chdir(root)
	code, _, errOut := run(t, "", "apply", "a.md", "--type", "API",
		"--tag", "設計", "--tag", "api", "--write")
	if code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, errOut)
	}
	want := "---\ntype: API\ntags:\n  - 設計\n  - api\n---\n\n本文\n"
	if got := read(t, root, "a.md"); got != want {
		t.Errorf("a.md = %q", got)
	}
	if strings.Contains(errOut, "dry-run") {
		t.Errorf("--write で dry-run の注意が出ている: %q", errOut)
	}
}

func TestApply_json(t *testing.T) {
	root := tree(t, map[string]string{"a.md": "---\ntype: API\n---\n本文\n"})
	t.Chdir(root)
	code, out, _ := run(t, "", "apply", "a.md", "--tag", "x", "--tag", "y",
		"--format", "json", "--mode", "merge")
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	var rep struct {
		DryRun  bool           `json:"dry_run"`
		Summary map[string]int `json:"summary"`
		Scope   struct {
			Paths     []string `json:"paths"`
			Recursive bool     `json:"recursive"`
		} `json:"scope"`
		Files []struct {
			Path    string   `json:"path"`
			Action  string   `json:"action"`
			Added   []string `json:"added"`
			Errors  []any    `json:"errors"`
			Changes []struct {
				Key    string `json:"key"`
				Before any    `json:"before"`
				After  any    `json:"after"`
			} `json:"changes"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("JSON として読めない: %v\n%s", err, out)
	}
	if !rep.DryRun || rep.Summary["update"] != 1 || !rep.Scope.Recursive || rep.Scope.Paths[0] != "a.md" {
		t.Errorf("rep = %+v", rep)
	}
	f := rep.Files[0]
	if f.Action != "update" || f.Added[0] != "tags" || f.Errors == nil || len(f.Changes) != 1 || f.Changes[0].Key != "tags" {
		t.Errorf("file = %+v", f)
	}
}

func TestApply_warningInTable(t *testing.T) {
	root := tree(t, map[string]string{"a.md": "---\ntype: API\n---\n"})
	t.Chdir(root)
	m := `{"version":1,"files":[{"path":"a.md","frontmatter":{"tag":"x"}}]}`
	code, out, errOut := run(t, m, "apply", "--manifest", "-", "--verbose")
	if code != 0 {
		t.Errorf("警告だけなら 0 のはず: %d", code)
	}
	if !strings.Contains(out, "+tag warn: legacy_key") {
		t.Errorf("stdout = %q", out)
	}
	if !strings.Contains(errOut, "警告 a.md: [legacy_key]") {
		t.Errorf("--verbose の stderr = %q", errOut)
	}
}

func TestApply_usageErrors(t *testing.T) {
	root := tree(t, map[string]string{"a.md": "x"})
	t.Chdir(root)
	tests := map[string][]string{
		"対象なし":       {"apply", "--type", "API"},
		"不正な format": {"apply", "a.md", "--format", "xml"},
		"不正な mode":   {"apply", "a.md", "--mode", "x"},
		"不正な --set":  {"apply", "a.md", "--set", "novalue"},
		"マニフェストがない":  {"apply", "--manifest", filepath.Join(root, "nothing.json")},
		"不明なフラグ":     {"apply", "a.md", "--nothing"},
		"不明なコマンド":    {"nothing"},
		"コマンドなし":     {},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			if code, _, _ := run(t, "", args...); code != 2 {
				t.Errorf("code = %d, want 2", code)
			}
		})
	}
	if code, _, _ := run(t, `{"version":9,"files":[]}`, "apply", "--manifest", "-"); code != 2 {
		t.Errorf("不正なマニフェスト: code = %d, want 2", code)
	}
}

func TestVersionAndHelp(t *testing.T) {
	if code, out, _ := run(t, "", "version"); code != 0 || out != "okf-stamp dev\n" {
		t.Errorf("version: %d %q", code, out)
	}
	if code, out, _ := run(t, "", "help"); code != 0 || !strings.Contains(out, "okf-stamp apply") {
		t.Errorf("help: %d %q", code, out)
	}
	if code, _, _ := run(t, "", "apply", "-h"); code != 0 {
		t.Errorf("apply -h: %d", code)
	}
}
