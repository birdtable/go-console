package ui

// The one document a run answers with under --json.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// resultVersion is the shape of Result: it moves when a field is added,
// renamed, removed or read another way. 1 is version, word, what, next, code,
// changes and error; 2 adds dry_run; 3 makes error a Fault and adds command,
// apply, run and data.
const resultVersion = 3

// ErrResultVersion is the error ReadResult answers with for a document of
// another version than this one reads.
var ErrResultVersion = errors.New("ui: the document is not a version this reader reads")

// A Fault is the failure a run stopped on, as a script reads it: Kind is the
// word of the ending the run closed on, Msg the error's own text, Hint the
// line to type next, or empty.
type Fault struct {
	// Kind is the closing word the run ended on ("failed", "refused"): the
	// word of the Ending the command returned.
	Kind string `json:"kind"`
	Msg  string `json:"msg"`
	Hint string `json:"hint"`
}

// A Result is what a run says under --json when its command printed no
// document of its own: how it ended, what it changed or would change, and
// what failed. Execute prints it. Command, Apply, Run, Data
// and Error are absent from the document, not null, when they are not set.
type Result struct {
	Version int `json:"version"`
	// Command is the path of the command that ran ("app sync"), as Execute
	// found it.
	Command string `json:"command,omitempty"`
	// Word is the closing word the run ended on, or empty before it ended.
	Word string `json:"word"`
	What string `json:"what"`
	// Next is the line to type, or empty. It is the ending's, set by the
	// command; it is never found in What.
	Next string `json:"next"`
	// Code is the exit code: the ending's, a failure's with no ending, or 0.
	Code int `json:"code"`
	// Changes is every change the run reported, the plan under a dry run.
	Changes []string `json:"changes"`
	// DryRun is whether the run was a dry run, so Changes is a plan and
	// nothing was changed: true unless the program applied
	// (Session.SetDryRun(false)), as a session's zero value is.
	DryRun bool `json:"dry_run"`
	// Apply is whether the program said, with Session.SetDryRun, that its
	// changes were made (true) or planned (false), and absent when it said
	// neither.
	Apply *bool `json:"apply,omitempty"`
	// Run is the id of the run's record, set with Session.SetRun.
	Run string `json:"run,omitempty"`
	// Data is the command's own data, set with Session.SetData.
	Data any `json:"data,omitempty"`
	// Error is the failure the run stopped on, or nil.
	Error *Fault `json:"error,omitempty"`
}

// Result is what the session answers with: its ending, the run's changes, and
// err, the error its command returned. Under Execute every error ends on an
// ending first, so the words are the ones the text form prints; an error with
// no ending answers with a failure's code and, as its kind, the word of the
// session's failed ending.
func (s *Session) Result(err error) Result {
	a := Result{Version: resultVersion, Changes: append([]string{}, s.Changes()...), DryRun: s.DryRun()}
	if p := s.path.Load(); p != nil {
		a.Command = *p
	}
	if s.dryRunSet.Load() {
		apply := !s.DryRun()
		a.Apply = &apply
	}
	s.mu.Lock()
	a.Run, a.Data = s.runID, s.data
	s.mu.Unlock()
	e := s.Ended()
	if e != nil {
		a.Word, a.What, a.Next, a.Code = e.Word(), e.What(), e.Next(), e.Code()
	}
	if s.own.Load() {
		a.Code = s.ExitCode()
		err = nil
	}
	if err != nil {
		a.Error = &Fault{Msg: err.Error()}
		if e != nil {
			a.Error.Kind, a.Error.Hint = e.Word(), e.Next()
		} else if s.handled.Load() {
			a.Code = s.ExitCode()
		} else {
			a.Code = s.failedCode()
			if s.hasEnds {
				a.Error.Kind = s.failedKind().Word
			}
		}
	}
	return a
}

// ReadResult reads one document a run printed under --json. It refuses a
// document whose version is not this one's with ErrResultVersion, so a
// reader of version 3 never takes a version 2 error string for an object.
func ReadResult(b []byte) (Result, error) {
	var head struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(b, &head); err != nil {
		return Result{}, fmt.Errorf("ui: the document is not JSON: %w", err)
	}
	if head.Version != resultVersion {
		return Result{}, fmt.Errorf("%w: it is version %d, and this reads version %d", ErrResultVersion, head.Version, resultVersion)
	}
	var r Result
	if err := json.Unmarshal(b, &r); err != nil {
		return Result{}, fmt.Errorf("ui: the document is not a version %d result: %w", resultVersion, err)
	}
	return r, nil
}

// Write prints r as one indented JSON document and a newline.
func (r Result) Write(w io.Writer) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("ui: the answer cannot be written as JSON: %w", err)
	}
	_, err = w.Write(append(b, '\n'))
	return err
}
