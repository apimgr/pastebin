//go:build gui && windows

package gui

import "golang.org/x/sys/windows/registry"

// systemPrefersDark detects Windows dark mode via the
// HKCU\...\Personalize\AppsUseLightTheme registry value (AI.md "System
// Theme Detection"): 0 = dark, 1 = light. Any read failure (older Windows
// without the key, permission error) falls back to the dark default.
func systemPrefersDark() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return true
	}
	defer k.Close()

	v, _, err := k.GetIntegerValue("AppsUseLightTheme")
	if err != nil {
		return true
	}
	return v == 0
}
