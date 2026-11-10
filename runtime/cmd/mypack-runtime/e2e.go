//go:build e2e

package main

// Test builds only (never shipped): lets `mygo-automation` drive the app when
// MYGO_AUTOMATION=<socket> is set. Build with: go build -tags e2e ...
import _ "github.com/egoist/mygo/plugins/automation"
