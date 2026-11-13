package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture は検証用のバンドルを作り、そのパスを返す。
func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"api/auth.md":     "---\ntype: API\ntitle: 認証API\ndescription: ログインとトークン発行\ntags: [設計/基本設計, api]\n---\n本文\n",
		"screen/login.md": "---\ntype: 画面\ntitle: ログイン画面\ntags: [設計/基本設計]\naliases: [認証画面]\n---\n",
		"glossary/jwt.md": "---\ntype: 用語\ntitle: JWT\nstatus: draft\n---\n",
		"glossary/api.md": "---\ntype: api\ntitle: API とは\n---\n",
		"index.md":        "---\nokf_version: \"0.2\"\n---\n",
		"README.md":       "# frontmatter なし\n",
		"bad.md":          "---\ntype: X\ntags: [2026, \"a b\", ok]\n---\n",
	}
	for rel, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = Run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestSearch_table(t *testing.T) {
	dir := fixture(t)
	code, out, errOut := run(t, "search", "認証", "--dir", dir)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, errOut)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %q", lines)
	}
	// 列は TYPE, TITLE, PATH の順。
	if !strings.HasPrefix(lines[0], "TYPE") || strings.Index(lines[0], "TITLE") > strings.Index(lines[0], "PATH") {
		t.Errorf("header = %q", lines[0])
	}
	// title 完全一致ではなく部分一致(60)の「認証API」が、aliases(40)の「ログイン画面」より前。
	if !strings.Contains(lines[1], "認証API") || !strings.Contains(lines[2], "ログイン画面") {
		t.Errorf("順序が違う: %q", lines)
	}
	if !strings.Contains(lines[1], "api/auth.md") {
		t.Errorf("path がない: %q", lines[1])
	}
	if !strings.Contains(errOut, "2 件") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestSearch_tableAlignsWideCharacters(t *testing.T) {
	dir := fixture(t)
	_, out, _ := run(t, "search", "--dir", dir, "--type", "API", "--type", "画面")
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	// 全角を 2 桁と数えて、TITLE 列の開始位置が全行で揃う。
	pos := -1
	for _, l := range lines {
		i := strings.Index(l, "認証API")
		if i < 0 {
			i = strings.Index(l, "ログイン画面")
		}
		if i < 0 {
			i = strings.Index(l, "TITLE")
		}
		if i < 0 {
			continue
		}
		w := displayWidth(l[:i])
		if pos == -1 {
			pos = w
		} else if w != pos {
			t.Errorf("TITLE 列の位置が揃わない: %d と %d\n%s", pos, w, out)
		}
	}
}

func TestSearch_flagsAfterQuery(t *testing.T) {
	dir := fixture(t)
	code, out, _ := run(t, "search", "認証", "--type", "API", "--dir", dir)
	if code != 0 || !strings.Contains(out, "認証API") || strings.Contains(out, "ログイン画面") {
		t.Errorf("code = %d, out = %q", code, out)
	}
}

func TestSearch_multiWordQueryWithoutQuotes(t *testing.T) {
	dir := fixture(t)
	_, out, _ := run(t, "search", "認証", "API", "--dir", dir)
	if !strings.Contains(out, "認証API") || strings.Contains(out, "ログイン画面") {
		t.Errorf("out = %q", out)
	}
}

func TestSearch_json(t *testing.T) {
	dir := fixture(t)
	code, out, _ := run(t, "search", "--dir", dir, "--tag", "設計", "--format", "json")
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("JSON として読めない: %v\n%s", err, out)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d 件", len(items))
	}
	if items[0]["path"] != "api/auth.md" || items[0]["type"] != "API" {
		t.Errorf("items[0] = %v", items[0])
	}
	for _, key := range []string{"title", "description", "tags", "aliases", "score"} {
		if _, ok := items[0][key]; !ok {
			t.Errorf("%s がない", key)
		}
	}
}

func TestSearch_jsonEmptyIsArray(t *testing.T) {
	dir := fixture(t)
	code, out, _ := run(t, "search", "存在しない語", "--dir", dir, "--format", "json")
	if code != 0 || strings.TrimSpace(out) != "[]" {
		t.Errorf("code = %d, out = %q", code, out)
	}
}

func TestSearch_noHitsExitsZero(t *testing.T) {
	dir := fixture(t)
	code, out, errOut := run(t, "search", "存在しない語", "--dir", dir)
	if code != 0 || out != "" || !strings.Contains(errOut, "0 件") {
		t.Errorf("code = %d, out = %q, stderr = %q", code, out, errOut)
	}
}

func TestSearch_where(t *testing.T) {
	dir := fixture(t)
	_, out, _ := run(t, "search", "--dir", dir, "--where", "status=draft")
	if !strings.Contains(out, "JWT") || strings.Contains(out, "認証API") {
		t.Errorf("out = %q", out)
	}
}

func TestSearch_skippedSummaryAndVerbose(t *testing.T) {
	dir := fixture(t)
	_, _, errOut := run(t, "search", "--dir", dir)
	// README.md は frontmatter なしで読み飛ばし、bad.md は不正なタグが 2 件。
	if !strings.Contains(errOut, "1 件のファイルを読み飛ばした") || !strings.Contains(errOut, "警告は 2 件") {
		t.Errorf("stderr = %q", errOut)
	}
	if strings.Contains(errOut, "README.md") {
		t.Errorf("通常は詳細を出さない: %q", errOut)
	}

	_, _, errOut = run(t, "search", "--dir", dir, "--verbose")
	if !strings.Contains(errOut, "読み飛ばし: README.md") || !strings.Contains(errOut, "警告: bad.md") {
		t.Errorf("verbose の stderr = %q", errOut)
	}
}

func TestSearch_excludesIndex(t *testing.T) {
	dir := fixture(t)
	_, out, _ := run(t, "search", "--dir", dir)
	if strings.Contains(out, "index.md") {
		t.Errorf("index.md が含まれる: %q", out)
	}
}

func TestSearch_usageErrors(t *testing.T) {
	dir := fixture(t)
	tests := [][]string{
		{"search", "--dir", dir, "--format", "xml"},
		{"search", "--dir", dir, "--where", "novalue"},
		{"search", "--dir", dir, "--where", "=x"},
		{"search", "--dir", dir, "--unknown"},
	}
	for _, args := range tests {
		if code, _, _ := run(t, args...); code != 2 {
			t.Errorf("%v: code = %d, want 2", args, code)
		}
	}
}

func TestSearch_missingDirExitsOne(t *testing.T) {
	code, _, errOut := run(t, "search", "--dir", filepath.Join(t.TempDir(), "missing"))
	if code != 1 || !strings.Contains(errOut, "エラー") {
		t.Errorf("code = %d, stderr = %q", code, errOut)
	}
}

func TestTypes(t *testing.T) {
	dir := fixture(t)
	code, out, _ := run(t, "types", "--dir", dir)
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	// API と api は表記ごとに別の行になる。
	for _, want := range []string{"TYPE", "COUNT", "API", "api", "画面", "用語"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q がない: %q", want, out)
		}
	}

	_, out, _ = run(t, "types", "--dir", dir, "--format", "json")
	var counts []typeCount
	if err := json.Unmarshal([]byte(out), &counts); err != nil {
		t.Fatalf("JSON として読めない: %v", err)
	}
	total := 0
	for _, c := range counts {
		total += c.Count
	}
	if total != 5 { // auth, login, jwt, api, bad
		t.Errorf("合計 = %d, want 5", total)
	}
}

func TestTypes_rejectsPositional(t *testing.T) {
	if code, _, _ := run(t, "types", "余計な引数"); code != 2 {
		t.Errorf("code = %d, want 2", code)
	}
}

func TestRun_commands(t *testing.T) {
	if code, out, _ := run(t, "help"); code != 0 || !strings.Contains(out, "okf-lens search") {
		t.Errorf("help: code = %d, out = %q", code, out)
	}
	if code, _, errOut := run(t); code != 2 || !strings.Contains(errOut, "使い方") {
		t.Errorf("引数なし: code = %d, stderr = %q", code, errOut)
	}
	if code, _, errOut := run(t, "unknown"); code != 2 || !strings.Contains(errOut, "不明なコマンド") {
		t.Errorf("不明: code = %d, stderr = %q", code, errOut)
	}
	if code, _, _ := run(t, "search", "-h"); code != 0 {
		t.Errorf("search -h: code = %d, want 0", code)
	}
}

func TestSearch_exclude(t *testing.T) {
	dir := fixture(t)
	code, out, errOut := run(t, "search", "--dir", dir, "--exclude", "README.md", "--exclude", "bad.md", "--exclude", "glossary/")
	if code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, errOut)
	}
	if strings.Contains(out, "bad.md") || strings.Contains(out, "glossary") {
		t.Errorf("除外されていない: %q", out)
	}
	// README.md と bad.md を除外したので、読み飛ばしも警告も出ない。
	if strings.Contains(errOut, "読み飛ばした") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestSearch_ignoreFile(t *testing.T) {
	dir := fixture(t)
	if err := os.WriteFile(filepath.Join(dir, ".okf-lensignore"), []byte("README.md\nbad.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := run(t, "search", "--dir", dir)
	if code != 0 || strings.Contains(errOut, "読み飛ばした") {
		t.Errorf("code = %d, stderr = %q", code, errOut)
	}
	// types にも同じ除外が効く。
	_, out, _ := run(t, "types", "--dir", dir, "--format", "json")
	var counts []typeCount
	if err := json.Unmarshal([]byte(out), &counts); err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, c := range counts {
		total += c.Count
	}
	if total != 4 { // bad.md を除いた 4 件
		t.Errorf("合計 = %d, want 4", total)
	}
}

func TestSearch_invalidExcludeExitsTwo(t *testing.T) {
	dir := fixture(t)
	code, _, errOut := run(t, "search", "--dir", dir, "--exclude", "!a.md")
	if code != 2 || !strings.Contains(errOut, "--exclude") {
		t.Errorf("code = %d, stderr = %q", code, errOut)
	}
}

// linkFixture はリンクを持つバンドルを作る。
//
//	login → auth → jwt
//	login → jwt
//	batch → auth(frontmatter の computation)
//	auth → missing(壊れたリンク)
func linkFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"api/auth.md":     "---\ntype: API\ntitle: 認証API\n---\n[JWT](/glossary/jwt.md) [壊れ](/missing.md)\n",
		"screen/login.md": "---\ntype: 画面\ntitle: ログイン画面\n---\n[認証](../api/auth.md) [JWT](/glossary/jwt.md)\n",
		"glossary/jwt.md": "---\ntype: 用語\ntitle: JWT\n---\nリンクなし\n",
		"batch/sync.md":   "---\ntype: バッチ\ntitle: 同期\ncomputation: ../api/auth.md\n---\n",
		"index.md":        "---\nokf_version: \"0.2\"\n---\n[全部](/api/auth.md) [全部](/glossary/jwt.md)\n",
	}
	for rel, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func resultPaths(t *testing.T, out string) []string {
	t.Helper()
	var items []map[string]any
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("JSON として読めない: %v\n%s", err, out)
	}
	var paths []string
	for _, it := range items {
		paths = append(paths, it["path"].(string))
	}
	return paths
}

func TestSearch_linksTo(t *testing.T) {
	dir := linkFixture(t)
	tests := []struct {
		name string
		args []string
		want []string
	}{
		// index.md は jwt.md にリンクするが、リンク元として数えない。
		{"backlinks", []string{"--links-to", "glossary/jwt.md"}, []string{"api/auth.md", "screen/login.md"}},
		{"本文と frontmatter の両方", []string{"--links-to", "api/auth.md"}, []string{"batch/sync.md", "screen/login.md"}},
		{".md と先頭 / を省略", []string{"--links-to", "/glossary/jwt"}, []string{"api/auth.md", "screen/login.md"}},
		{"大文字小文字を区別しない", []string{"--links-to", "Glossary/JWT.md"}, []string{"api/auth.md", "screen/login.md"}},
		{"他の条件と組み合わせる", []string{"--links-to", "glossary/jwt.md", "--type", "画面"}, []string{"screen/login.md"}},
		{"複数指定は AND", []string{"--links-to", "glossary/jwt.md", "--links-to", "api/auth.md"}, []string{"screen/login.md"}},
		{"リンクされていない文書", []string{"--links-to", "batch/sync.md"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"search", "--dir", dir, "--format", "json"}, tt.args...)
			code, out, errOut := run(t, args...)
			if code != 0 {
				t.Fatalf("code = %d, stderr = %s", code, errOut)
			}
			if got := resultPaths(t, out); !equalStrings(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSearch_linkedFrom(t *testing.T) {
	dir := linkFixture(t)
	tests := []struct {
		name string
		args []string
		want []string
	}{
		// 壊れたリンク(missing.md)は、文書がないので結果に出ない。
		{"リンク先", []string{"--linked-from", "api/auth.md"}, []string{"glossary/jwt.md"}},
		{"複数のリンク先", []string{"--linked-from", "screen/login"}, []string{"api/auth.md", "glossary/jwt.md"}},
		{"複数指定は AND", []string{"--linked-from", "screen/login.md", "--linked-from", "batch/sync.md"}, []string{"api/auth.md"}},
		{"type と組み合わせる", []string{"--linked-from", "screen/login.md", "--type", "用語"}, []string{"glossary/jwt.md"}},
		{"links-to との併用", []string{"--linked-from", "screen/login.md", "--links-to", "batch/sync.md"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"search", "--dir", dir, "--format", "json"}, tt.args...)
			code, out, errOut := run(t, args...)
			if code != 0 {
				t.Fatalf("code = %d, stderr = %s", code, errOut)
			}
			if got := resultPaths(t, out); !equalStrings(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSearch_linksToBrokenTarget(t *testing.T) {
	dir := linkFixture(t)
	// missing.md は存在しないが、リンク元は探せる。注意は出すが、エラーにはしない。
	code, out, errOut := run(t, "search", "--dir", dir, "--format", "json", "--links-to", "missing.md")
	if code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, errOut)
	}
	if got := resultPaths(t, out); !equalStrings(got, []string{"api/auth.md"}) {
		t.Errorf("got %v", got)
	}
	if !strings.Contains(errOut, "注意") || !strings.Contains(errOut, "missing.md") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestSearch_linkedFromMissingSource(t *testing.T) {
	dir := linkFixture(t)
	code, out, errOut := run(t, "search", "--dir", dir, "--format", "json", "--linked-from", "nothing.md")
	if code != 0 || strings.TrimSpace(out) != "[]" || !strings.Contains(errOut, "注意") {
		t.Errorf("code = %d, out = %q, stderr = %q", code, out, errOut)
	}
}

func TestSearch_noLinkFlagsSkipsGraph(t *testing.T) {
	// リンクのフラグがなければ、これまでどおり全件を返す。
	dir := linkFixture(t)
	_, out, _ := run(t, "search", "--dir", dir, "--format", "json")
	if got := resultPaths(t, out); len(got) != 4 {
		t.Errorf("got %v", got)
	}
}

func TestVersion(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })
	Version = "1.2.3"
	for _, arg := range []string{"version", "--version", "-v"} {
		code, out, _ := run(t, arg)
		if code != 0 || out != "okf-lens 1.2.3\n" {
			t.Errorf("%s: code = %d, out = %q", arg, code, out)
		}
	}
}
