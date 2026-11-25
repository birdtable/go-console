package shell

import (
	"fmt"
	"strconv"
	"strings"
)

// String is the name of d: posix, powershell, cmd or windows. Any other value is Dialect(n), and String never panics.
func (d Dialect) String() string {
	switch d {
	case POSIX:
		return "posix"
	case PowerShell:
		return "powershell"
	case Cmd:
		return "cmd"
	case Windows:
		return "windows"
	}
	return "Dialect(" + strconv.Itoa(int(d)) + ")"
}

// Dialects is the four dialects, POSIX, PowerShell, Cmd and Windows, in that order. Each call returns a new slice.
func Dialects() []Dialect {
	return []Dialect{POSIX, PowerShell, Cmd, Windows}
}

// ParseDialect is the Dialect named by name: posix, powershell, cmd or windows, in any case, and nothing trimmed.
// Any other name, the empty one included, is 0 and an error that wraps ErrDialect and lists the names.
func ParseDialect(name string) (Dialect, error) {
	var names []string
	for _, d := range Dialects() {
		if strings.EqualFold(name, d.String()) {
			return d, nil
		}
		names = append(names, d.String())
	}
	return 0, fmt.Errorf("%w %q, want %s or %s", ErrDialect, name, strings.Join(names[:len(names)-1], ", "), names[len(names)-1])
}
