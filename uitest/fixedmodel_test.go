package uitest_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/birdtable/go-console/uitest"
)

// fixedModel draws the same two rows, the first in bold and red, whatever size or system it runs on.
type fixedModel struct{}

func (fixedModel) Init() tea.Cmd          { return nil }
func (fixedModel) Update(tea.Msg) tea.Cmd { return nil }
func (fixedModel) View() string {
	return "\x1b[1;31mdeploy\x1b[0m  ok   12 files\nnext     3 waiting"
}

// A small fixed model drawn at a fixed size is compared through uitest.Golden with one checked-in file: the styling is
// stripped and the trailing spaces and newlines are trimmed before the compare, so the same bytes are expected on Linux,
// macOS and Windows. The golden's line ends are LF in the repository and on every checkout (.gitattributes).
//
// What it costs when this fails: a golden a product checks in passes on the system its author used and fails on another,
// or a Windows checkout turns its line ends into CR LF and every golden there is red.
func TestAFixedModelIsTheSameGoldenOnEverySystem(t *testing.T) {
	t.Parallel()
	res := uitest.Run(uitest.ModelScreen(fixedModel{}), 30, 3)
	uitest.Golden(t, "fixed-model", res.Frame)
}
