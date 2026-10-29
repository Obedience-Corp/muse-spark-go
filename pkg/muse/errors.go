package muse

import (
	"encoding/json"
	"fmt"
)

// Error identifies a local, transport, process, compatibility or remote failure.
type Error struct {
	Kind     string
	Message  string
	Code     int
	Data     json.RawMessage
	ExitCode int
	Err      error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("muse %s: %s: %v", e.Kind, e.Message, e.Err)
	}
	return fmt.Sprintf("muse %s: %s", e.Kind, e.Message)
}
func (e *Error) Unwrap() error { return e.Err }
