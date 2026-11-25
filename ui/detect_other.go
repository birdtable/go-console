//go:build !windows

package ui

// unicodeOf is whether the terminal draws UTF-8, as the locale names say.
func unicodeOf(env []string, rules EnvRules) bool { return localeIsUTF8(env, rules) }
