// Command mkicns converts a square PNG into a macOS .icns file.
package main

import (
	"fmt"
	"os"

	"mypack.dev/mypack/internal/icons"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: mkicns <in.png> <out.icns>")
		os.Exit(2)
	}
	b, err := os.ReadFile(os.Args[1])
	if err == nil {
		b, err = icons.ICNS(b)
	}
	if err == nil {
		err = os.WriteFile(os.Args[2], b, 0o644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
