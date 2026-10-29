package muse

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Event retains the complete raw runtime record for forward compatibility.
type Event struct {
	Type    string          `json:"payload_type"`
	Payload json.RawMessage `json:"payload"`
	Raw     json.RawMessage `json:"-"`
}
type Result struct {
	Text      string
	SessionID string
	Terminal  string
	Raw       json.RawMessage
}

// StreamPrompt emits runtime records until process exit. Drain both channels or cancel ctx.
func (c *Client) StreamPrompt(ctx context.Context, prompt string, opts *RunOptions) (<-chan Event, <-chan error) {
	events := make(chan Event, 32)
	errs := make(chan error, 1)
	var o RunOptions
	if opts != nil {
		o = *opts
		o.Config = append([]string(nil), opts.Config...)
		o.Images = append([]string(nil), opts.Images...)
	}
	go func() {
		defer close(events)
		defer close(errs)
		args, err := o.args()
		if err != nil {
			errs <- &Error{Kind: "validation", Message: "options", Err: err}
			return
		}
		if o.PromptFile != "" && prompt != "" {
			errs <- &Error{Kind: "validation", Message: "prompt and prompt file are mutually exclusive"}
			return
		}
		if o.PromptFile == "" {
			args = append(args, "--", prompt)
		}
		cmd := c.command(ctx, args...)
		cmd.Stdin = o.APIKey
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			errs <- err
			return
		}
		cmd.Stderr = c.Stderr
		if err = cmd.Start(); err != nil {
			errs <- &Error{Kind: "process", Message: "start exec", Err: err}
			return
		}
		// A descendant may retain stdout after the owned process exits. Closing
		// our read end on cancellation keeps Scan from waiting on that descendant.
		stopRead := context.AfterFunc(ctx, func() { _ = stdout.Close() })
		defer stopRead()
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 65536), maxFrameBytes)
		terminalSeen := false
		for scanner.Scan() {
			raw := append([]byte(nil), scanner.Bytes()...)
			var e Event
			if err = json.Unmarshal(raw, &e); err != nil {
				break
			}
			if e.Type == "" || len(e.Payload) == 0 {
				err = fmt.Errorf("missing event type or payload")
				break
			}
			if terminalSeen && isTerminalEvent(e.Type) {
				err = fmt.Errorf("multiple terminal records")
				break
			}
			if isTerminalEvent(e.Type) {
				var terminal struct {
					Value string `json:"terminal"`
				}
				if err = json.Unmarshal(e.Payload, &terminal); err != nil {
					break
				}
				if terminal.Value == "" || e.Type != "run.terminal."+terminal.Value {
					err = fmt.Errorf("inconsistent terminal payload")
					break
				}
				terminalSeen = true
			}
			e.Raw = raw
			select {
			case events <- e:
			case <-ctx.Done():
				err = ctx.Err()
			}
			if err != nil {
				break
			}
		}
		if err == nil {
			err = scanner.Err()
		}
		if err != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		waitErr := cmd.Wait()
		if ctx.Err() != nil {
			errs <- ctx.Err()
			return
		}
		if err != nil {
			errs <- &Error{Kind: "protocol", Message: "decode exec stream", Err: err}
			return
		}
		if waitErr == nil && !terminalSeen {
			errs <- &Error{Kind: "protocol", Message: "exec exited without a terminal record"}
			return
		}
		if waitErr != nil {
			code := -1
			if ex, ok := waitErr.(*exec.ExitError); ok {
				code = ex.ExitCode()
			}
			errs <- &Error{Kind: "process", Message: "exec failed", ExitCode: code, Err: waitErr}
		}
	}()
	return events, errs
}
func (c *Client) RunPromptCtx(ctx context.Context, prompt string, opts *RunOptions) (*Result, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ev, er := c.StreamPrompt(ctx, prompt, opts)
	result := &Result{}
	found := false
	for e := range ev {
		if isTerminalEvent(e.Type) {
			var p struct {
				Text     string `json:"text"`
				Terminal string `json:"terminal"`
			}
			if err := json.Unmarshal(e.Payload, &p); err != nil {
				return nil, err
			}
			if p.Terminal == "" || e.Type != "run.terminal."+p.Terminal {
				return nil, &Error{Kind: "protocol", Message: "inconsistent terminal payload"}
			}
			result.Text = p.Text
			result.Terminal = p.Terminal
			result.Raw = e.Raw
			var wire struct {
				Stream struct {
					ID string `json:"id"`
				} `json:"stream"`
			}
			_ = json.Unmarshal(e.Raw, &wire)
			result.SessionID = wire.Stream.ID
			found = true
		}
	}
	if err := <-er; err != nil {
		if found {
			return result, err
		}
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("muse exec: no terminal record")
	}
	if result.Terminal != "completed" {
		return result, &Error{Kind: "remote", Message: "turn " + result.Terminal}
	}
	return result, nil
}

func isTerminalEvent(kind string) bool { return strings.HasPrefix(kind, "run.terminal.") }
