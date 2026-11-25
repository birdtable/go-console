package shell_test

import (
	"fmt"

	"github.com/birdtable/go-console/shell"
)

// A setting that names a dialect is read with ParseDialect, and a refusal lists the names a person may use.
func ExampleParseDialect() {
	d, err := shell.ParseDialect("PowerShell")
	fmt.Println(d, err)
	line, _ := shell.Quote(d, []string{"echo", "it's"})
	fmt.Println(line)
	_, err = shell.ParseDialect("bash")
	fmt.Println(err)
	// Output:
	// powershell <nil>
	// echo 'it''s'
	// shell: unknown dialect "bash", want posix, powershell, cmd or windows
}

func ExampleDialects() {
	for _, d := range shell.Dialects() {
		fmt.Println(d)
	}
	// Output:
	// posix
	// powershell
	// cmd
	// windows
}
