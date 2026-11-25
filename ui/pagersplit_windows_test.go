package ui

import (
	"slices"
	"testing"
)

// $PAGER is split by the Windows command-line rules, so a quoted path with spaces is one word.
//
// What it costs when this fails: a pager under C:\Program Files is run as C:\Program, and the page prints unpaged.
func TestThePagerOnWindowsIsSplitByTheCommandLineRules(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		pager string
		want  []string
	}{
		{`"C:\Program Files\Git\usr\bin\less.exe" -R`, []string{`C:\Program Files\Git\usr\bin\less.exe`, "-R"}},
		{`less -FRX`, []string{"less", "-FRX"}},
		{`C:\tools\more.exe`, []string{`C:\tools\more.exe`}},
		{`"C:\Program Files\bat\bat.exe" --paging=always`, []string{`C:\Program Files\bat\bat.exe`, "--paging=always"}},
	} {
		if got := PagerFromEnv([]string{"PAGER=" + c.pager}); !slices.Equal(got, c.want) {
			t.Errorf("PAGER=%s: the pager is %q, want %q", c.pager, got, c.want)
		}
	}
}
