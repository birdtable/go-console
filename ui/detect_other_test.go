//go:build !windows

package ui

import "testing"

// Detect says Unicode from the locale where there is no code page, through the names its rules give.
//
// What it costs when this fails: Detect leaves Unicode at its zero, so every terminal draws ASCII.
func TestDetectSetsUnicodeFromTheLocale(t *testing.T) {
	t.Parallel()
	for env, want := range map[string]bool{"LANG=en_US.UTF-8": true, "LANG=C": false, "": false} {
		var e []string
		if env != "" {
			e = []string{env}
		}
		if got := Detect(nil, nil, nil, e, StandardEnv()).Unicode; got != want {
			t.Errorf("Detect with %q: Unicode is %v, want %v", env, got, want)
		}
	}
	if Detect(nil, nil, nil, []string{"LANG=en_US.UTF-8"}, EnvRules{}).Unicode {
		t.Error("Detect read the locale under rules that name none")
	}
}
