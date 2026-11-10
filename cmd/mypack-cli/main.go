// Command mypack-cli is the headless mypack command line (no GUI).
package main

import (
	"os"

	"mypack.dev/mypack/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
