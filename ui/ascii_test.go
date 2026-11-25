package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/birdtable/go-console/uitest"
)

// The locale says the terminal draws UTF-8 by the first of LC_ALL, LC_CTYPE and LANG that is set, in any case,
// with or without the hyphen; one that is set to something else is the answer, whatever the next says.
//
// What it costs when this fails: a terminal that draws UTF-8 gets the ASCII forms, or one that does not gets
// block characters it prints as question marks.
func TestTheLocaleSaysWhetherTheTerminalDrawsUTF8(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		env  []string
		want bool
	}{
		"nothing set":            {nil, false},
		"LANG utf-8":             {[]string{"LANG=en_US.UTF-8"}, true},
		"LANG utf8 lower":        {[]string{"LANG=C.utf8"}, true},
		"LANG latin":             {[]string{"LANG=en_US.ISO-8859-1"}, false},
		"LANG C":                 {[]string{"LANG=C"}, false},
		"LC_CTYPE beats LANG":    {[]string{"LC_CTYPE=C", "LANG=en_US.UTF-8"}, false},
		"LC_CTYPE utf over LANG": {[]string{"LC_CTYPE=de_DE.UTF-8", "LANG=C"}, true},
		"LC_ALL beats both":      {[]string{"LC_ALL=POSIX", "LC_CTYPE=en_US.UTF-8", "LANG=en_US.UTF-8"}, false},
		"LC_ALL utf over both":   {[]string{"LC_ALL=en_GB.UTF8", "LC_CTYPE=C", "LANG=C"}, true},
		"an empty one is unset":  {[]string{"LC_ALL=", "LANG=en_US.UTF-8"}, true},
	} {
		if got := localeIsUTF8(c.env, StandardEnv()); got != c.want {
			t.Errorf("%s: %v is %v, want %v", name, c.env, got, c.want)
		}
	}
	if localeIsUTF8([]string{"LANG=en_US.UTF-8"}, EnvRules{}) {
		t.Error("a name the rules leave empty was read")
	}
}

// Without Unicode the wordmark, the spinner, the meter and the header rule are drawn in ASCII, and with it they are
// not: the goldens are the ASCII forms.
//
// What it costs when this fails: a console on a legacy code page prints the banner, the spinner and the bars
// as question marks and boxes.
func TestWithoutUnicodeTheFormsAreASCII(t *testing.T) {
	t.Parallel()
	at := stream{rich: true, interactive: true, width: 80, ascii: true}
	b := Brand{Name: "tool", Mark: "(o)", Row: "a made-up tagline"}
	table := ansi.Strip(richTable(at, []string{"name", "state"}, [][]string{{"alpha", "running"}}, true))
	frames := ""
	for i := range 10 {
		frames += spinner(at, time.Duration(i)*80*time.Millisecond)
	}
	forms := ansi.Strip(banner(at, b, "v1.2.3")) + "\n" + table + "\n" + frames + "\n" + meterOn(at, 0.5, 8) + "\n" + meterOn(at, 1, 4)
	uitest.Golden(t, "ascii-forms", forms)
	for _, c := range forms {
		if c > 0x7e {
			t.Fatalf("a rune above ASCII, %q, in:\n%s", c, forms)
		}
	}
	if !strings.Contains(table, "---") || strings.Contains(banner(at, b, "v1"), "(o)") {
		t.Errorf("the rule is not a run of hyphens or the mark was drawn:\n%s", table)
	}
	uni := at
	uni.ascii = false
	if got := ansi.Strip(banner(uni, b, "v1")); !strings.Contains(got, "█") || !strings.Contains(got, "(o)") {
		t.Errorf("with Unicode the banner is not drawn in blocks with its mark:\n%s", got)
	}
	if spinner(uni, 0) != "⠋" || meterOn(uni, 1, 2) != "██" {
		t.Error("with Unicode the spinner or the meter is not drawn as before")
	}
}

// A session built On a zero Terminal draws ASCII, as a pipe does, and one On a Unicode terminal draws blocks.
//
// What it costs when this fails: the session ignores what Detect found, so the answer is read and never used.
func TestASessionDrawsTheFormsItsTerminalSays(t *testing.T) {
	t.Parallel()
	zero, uni := New(On(Terminal{Interactive: true})), New(On(Terminal{Interactive: true, Unicode: true}))
	if got := zero.Meter(1, 3); got != "###" {
		t.Errorf("a Terminal without Unicode draws the meter %q, want ###", got)
	}
	if got := uni.Meter(1, 3); got != "███" {
		t.Errorf("a Terminal with Unicode draws the meter %q, want blocks", got)
	}
}
