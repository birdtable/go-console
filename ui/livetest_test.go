package ui_test

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/go-console/internal/english"
	"github.com/birdtable/go-console/ui"
	"github.com/birdtable/go-console/uitest"
)

func TestADriverRunsALiveWithKeysAndAFakeClock(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	var fail error
	var stepped []string
	d := ui.New(ui.WithMessages(english.Messages())).NewLiveDriver(ui.Live{
		Frame:   ui.HouseFrame,
		Columns: []ui.TableColumn{{Title: "id", Priority: 9}, {Title: "state", Priority: 5}},
		Rows: func() ([][]string, error) {
			if fail != nil {
				return nil, fail
			}
			return [][]string{{"ac11", "running"}, {"b202", "stopped"}}, nil
		},
		Keys: []ui.LiveKey{{Key: "e", Help: "enter", Exec: func(it ui.LiveItem) *exec.Cmd {
			return exec.CommandContext(t.Context(), "app", "shell", it.ID)
		}}},
	}, func() time.Time { return now }, func(c *exec.Cmd) error {
		stepped = append(stepped, strings.Join(c.Args, " "))
		return nil
	})
	s := uitest.LiveScreen(d)

	// A key moves the selection; a key bound to Exec steps into the row.
	res := uitest.Run(s, 60, 20, "down", "e")
	uitest.Golden(t, "driver-stepped", res.Frame)
	if len(stepped) != 1 || stepped[0] != "app shell b202" || !strings.Contains(res.Frame, "back from b202") {
		t.Errorf("stepped into %v:\n%s", stepped, res.Frame)
	}

	// A tick after a failed read keeps the rows and dates them by the first read.
	fail, now = errors.New("box did not answer"), now.Add(time.Minute)
	res = uitest.Feed(s, d.Tick(now))
	uitest.Golden(t, "driver-unread", res.Frame)
	for _, want := range []string{"ac11", "box did not answer", "read at 12:00:00"} {
		if !strings.Contains(res.Frame, want) {
			t.Errorf("%q is missing after a failed read:\n%s", want, res.Frame)
		}
	}
	if strings.Contains(res.Frame, "12:01:00") {
		t.Errorf("a failed read was dated as read:\n%s", res.Frame)
	}
}

func TestADriverReportsAProgramThatFailed(t *testing.T) {
	t.Parallel()
	d := ui.New(ui.WithMessages(english.Messages())).NewLiveDriver(ui.Live{
		Frame:   ui.HouseFrame,
		Columns: []ui.TableColumn{{Title: "id", Priority: 9}},
		Rows:    func() ([][]string, error) { return [][]string{{"ac11"}}, nil },
		Keys: []ui.LiveKey{{Key: "e", Help: "enter", Exec: func(ui.LiveItem) *exec.Cmd {
			return exec.CommandContext(t.Context(), "app")
		}}},
	}, time.Now, func(*exec.Cmd) error { return errors.New("exit 2") })
	res := uitest.Run(uitest.LiveScreen(d), 60, 20, "e")
	if !strings.Contains(res.Frame, "back from ac11: exit 2") {
		t.Errorf("the program's failure was lost:\n%s", res.Frame)
	}
}
