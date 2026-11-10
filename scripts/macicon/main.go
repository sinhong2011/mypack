// Command macicon converts icon artwork into full-bleed macOS app-icon art.
package main

import (
	"fmt"
	"os"

	"mypack.dev/mypack/internal/icons"
)

func main() {
	b, err := os.ReadFile(os.Args[1])
	if err == nil {
		b, err = icons.MacAppIcon(b)
	}
	if err == nil {
		err = os.WriteFile(os.Args[2], b, 0o644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
