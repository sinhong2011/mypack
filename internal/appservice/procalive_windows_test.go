//go:build windows

package appservice

func processAlive(pid int) bool { return false }
