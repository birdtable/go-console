package uitest

import (
	"fmt"
	"os"
	"testing"

	"github.com/birdtable/go-testkit"
)

// TestMain starts the tests in an environment of their own, made by testkit: no setting of the person's, an agent's or git's
// reaches a test, and a go a test starts keeps the run's build cache and modules and fetches nothing.
func TestMain(m *testing.M) {
	code, err := testkit.Main(m, testkit.GoOffline())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}

// What it costs when this fails: a test reads or writes the person's own home, temp folder or settings, or runs with an
// agent's or git's variables, and passes or fails on what the machine holds.
func TestTheTestsRunInAnEnvironmentOfTheirOwn(t *testing.T) {
	t.Parallel()
	testkit.Isolated(t)
}
