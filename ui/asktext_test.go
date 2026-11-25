package ui

// The question fields, and the nil nobody can see fail.

import (
	"errors"
	"testing"
)

// What this costs when it fails: a panic at a real terminal, on a question with
// nothing to validate, that no test here can reach. huh calls the validator on
// every keystroke and never checks it for nil, and the seam these tests drive
// returns before huh is built at all.
func TestAQuestionWithNothingToCheckGetsAValidatorAnyway(t *testing.T) {
	t.Parallel()
	if got := orNoCheck(nil); got == nil {
		t.Fatal("a question with no validator would hand huh a nil")
	}
	if err := orNoCheck(nil)("anything at all"); err != nil {
		t.Errorf("the stand-in validator refused a value: %v", err)
	}
}

// And the half that fails if the stand-in is ever returned unconditionally,
// which would accept every malformed address somebody types.
func TestAQuestionWithAValidatorKeepsIt(t *testing.T) {
	t.Parallel()
	refuse := errors.New("no")
	got := orNoCheck(func(string) error { return refuse })
	if err := got("anything"); !errors.Is(err, refuse) {
		t.Errorf("the caller's validator was replaced: %v", err)
	}
}
