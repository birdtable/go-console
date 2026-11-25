package ui

// A secret a person hands over, such as a sign-in's code: read from
// standard input, never from an argument, and never shown. At a terminal
// it is typed with the echo off; otherwise it is the first line of what is
// piped in, which is what a script or a UI does.

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/x/term"
)

// Secret is one line of the session's input (In) that is never shown, at most
// most bytes, with the space around it cut. When the input is a terminal,
// prompt is put on the session's error stream and the line is typed with the echo off.
// Nothing typed is kept anywhere but the answer.
func (s *Session) Secret(prompt string, most int) (string, error) {
	in := s.inR()
	if f, ok := in.(fder); ok && isTerminal(in) {
		fmt.Fprint(s.errW(), prompt)
		b, err := term.ReadPassword(f.Fd())
		fmt.Fprintln(s.errW())
		if err != nil {
			return "", err
		}
		if len(b) > most {
			clear(b)
			return "", fmt.Errorf("that is over %d bytes", most)
		}
		s := strings.TrimSpace(string(b))
		clear(b)
		return s, nil
	}
	return firstLine(in, most)
}

func firstLine(in io.Reader, most int) (string, error) {
	r := bufio.NewReaderSize(io.LimitReader(in, int64(most)+2), most+2)
	line, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if len(strings.TrimRight(line, "\r\n")) > most {
		return "", fmt.Errorf("that is over %d bytes", most)
	}
	return strings.TrimSpace(line), nil
}

// PipedSecret is Secret for a run that asked for --json: it never prompts,
// since a program is never shown a form,
// so the secret must be on in, and in is refused when it is a terminal.
func PipedSecret(in io.Reader, most int) (string, error) { return pipedSecret(in, most, isTerminal) }

func pipedSecret(in io.Reader, most int, terminal func(any) bool) (string, error) {
	if terminal(in) {
		return "", errors.New("with JSON on nothing is asked: pipe the secret in on standard input")
	}
	return firstLine(in, most)
}
