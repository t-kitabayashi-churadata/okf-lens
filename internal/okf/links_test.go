package okf

import (
	"reflect"
	"testing"
)

func linksOf(t *testing.T, src string) []string {
	t.Helper()
	doc, err := Parse("a.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return ExtractLinks(doc)
}

func TestExtractLinks_body(t *testing.T) {
	src := "---\ntype: X\n---\n" +
		"[絶対](/tables/customers.md) と [相対](./other.md) と [親](../c/d.md)。\n" +
		"[見出し付き](x.md#section) と [参照形式][ref] と [空白](<my file.md>)。\n" +
		"[外部](https://example.com/a.md)\n\n" +
		"[ref]: refs/target.md\n"
	want := []string{
		"/tables/customers.md", "./other.md", "../c/d.md",
		"x.md#section", "refs/target.md", "my file.md",
		"https://example.com/a.md",
	}
	got := linksOf(t, src)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestExtractLinks_ignored(t *testing.T) {
	src := "---\ntype: X\n---\n" +
		"![画像](img.md)\n\n" +
		"```md\n[コードブロック](code.md)\n```\n\n" +
		"    [インデントされたコード](indented.md)\n\n" +
		"`[インラインコード](inline.md)`\n\n" +
		"<https://example.com/auto.md>\n\n" +
		"[本物](real.md)\n"
	if got, want := linksOf(t, src), []string{"real.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractLinks_frontmatter(t *testing.T) {
	src := `---
type: Attested Computation
resource: /datasets/sales.md
computation: ../computations/revenue.md
runtime: bigquery
executor:
  resource: executors/run.md
attester:
  resource: https://example.com/attest
sources:
  - resource: /sources/a.md
    title: A
  - resource: "project:all-queries"
  - title: リンクなし
---
本文 [x](body.md)
`
	got := linksOf(t, src)
	want := []string{
		"body.md",
		"/datasets/sales.md", "../computations/revenue.md",
		"executors/run.md", "https://example.com/attest",
		"/sources/a.md", "project:all-queries",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestExtractLinks_none(t *testing.T) {
	if got := linksOf(t, "---\ntype: X\n---\nリンクなし\n"); len(got) != 0 {
		t.Errorf("got %q", got)
	}
}
