//go:build e2e

package main

// Test builds only: lets mygo-automation drive the GUI (MYGO_AUTOMATION=<socket>).
import _ "github.com/egoist/mygo/plugins/automation"
