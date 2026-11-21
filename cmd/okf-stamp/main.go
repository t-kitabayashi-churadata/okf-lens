// okf-stamp は concept ファイルに OKF の frontmatter を付与する CLI。
package main

import (
	"os"

	"github.com/t-kitabayashi-churadata/okf-lens/internal/stamp/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
