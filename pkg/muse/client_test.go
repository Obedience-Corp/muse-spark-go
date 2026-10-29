package muse

import (
	"context"
	"errors"
	"testing"
)

func TestMissingBinary(t *testing.T) {
	_, err := NewClient("/nonexistent/muse").Version(context.Background())
	if err == nil {
		t.Fatal("expected missing binary error")
	}
}
func TestCancelledStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewClient("missing").StartMSP(ctx, HostOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}
func TestInvalidCommandID(t *testing.T) {
	if _, err := commandID("not-an-id"); err == nil {
		t.Fatal("expected invalid command id")
	}
}

func TestOptionValidation(t *testing.T) {
	for _, o := range []RunOptions{{WorktreeBase: "main"}, {Worktree: "existing"}, {Worktree: "bogus"}, {DisableRequestUserInput: true, UserInputAutoResolve: true}, {Images: []string{""}}, {Config: []string{""}}} {
		if _, err := o.args(); err == nil {
			t.Errorf("accepted invalid options %+v", o)
		}
	}
	if _, err := (HostOptions{UserInputDialogs: true, DisableRequestUserInput: true}).args(); err == nil {
		t.Fatal("contradictory input capabilities")
	}
}
