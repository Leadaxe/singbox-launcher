//go:build darwin

package ui

import "singbox-launcher/core"

// На macOS лаунчер умеет открыть Terminal.app с подставленной командой:
// пользователь видит полный вывод и вводит свой sudo сам.
func init() {
	openTerminal = func(cmd string) error {
		ac := core.GetController()
		if ac == nil {
			return nil
		}
		return ac.OpenTerminalWithCommand(cmd)
	}
}
