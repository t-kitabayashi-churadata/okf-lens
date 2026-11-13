// okf-lens は OKF のバンドルを検索する CLI。
package main

import (
	"os"

	"github.com/t-kitabayashi-churadata/okf-lens/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
