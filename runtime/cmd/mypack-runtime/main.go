// Command mypack-runtime is the single prebuilt binary every generated
// desktop app is made from. The Builder copies it into a bundle and injects
// mypack.json (see ARCHITECTURE.md, "Generated-app runtime contract").
package main

import "mypack.dev/mypack/runtime/app"

func main() { app.Main() }
