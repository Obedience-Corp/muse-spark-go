package muse

import (
	"context"
	"encoding/json"
	"sync"
)

// TurnOptions configures admission and optional event collection. CommandID may
// be supplied for deliberate value-identical retries; the SDK never retries.
type TurnOptions struct {
	CommandID       string
	IfBusy          string
	ReasoningEffort string
	DisplayText     string
	Stream          bool
	EventBuffer     int
}

// Turn tracks an admitted turn. Wait is repeatable and concurrent-safe. An
// application abandoning a turn should call CloseTracking (or close the host).
type Turn struct {
	host                                  *MSPHost
	SessionID, ID, Disposition, CommandID string
	sub                                   *Subscription
	done, stop                            chan struct{}
	stopOnce                              sync.Once
	events                                chan Notification
	result                                TurnResult
	err                                   error
}

func (s *Session) Send(ctx context.Context, input []InputPart) (*Turn, error) {
	return s.SendWithOptions(ctx, input, TurnOptions{})
}
func (s *Session) SendWithOptions(ctx context.Context, input []InputPart, o TurnOptions) (*Turn, error) {
	if len(input) == 0 {
		return nil, &Error{Kind: "validation", Message: "empty turn input"}
	}
	switch o.IfBusy {
	case "", "queue", "steer", "replace":
	default:
		return nil, &Error{Kind: "validation", Message: "invalid busy disposition"}
	}
	if o.EventBuffer < 0 {
		return nil, &Error{Kind: "validation", Message: "negative event buffer"}
	}
	id, err := commandID(o.CommandID)
	if err != nil {
		return nil, err
	}
	params := map[string]any{"commandId": id, "sessionId": s.Info.SessionID, "input": input}
	if o.IfBusy != "" {
		params["ifBusy"] = o.IfBusy
	}
	if o.ReasoningEffort != "" {
		params["reasoningEffort"] = o.ReasoningEffort
	}
	if o.DisplayText != "" {
		params["displayText"] = o.DisplayText
	}
	// Subscribe before admission: even a terminal may precede the response.
	sub := s.host.Subscribe(s.Info.SessionID, 512)
	var out struct {
		TurnID      string `json:"turnId"`
		Disposition string `json:"disposition"`
	}
	if err = s.host.Call(ctx, "turn/start", params, &out); err != nil {
		sub.Close()
		return nil, err
	}
	if out.TurnID == "" {
		sub.Close()
		return nil, &Error{Kind: "protocol", Message: "turn/start returned no turnId"}
	}
	size := o.EventBuffer
	if size == 0 {
		size = 256
	}
	t := &Turn{host: s.host, SessionID: s.Info.SessionID, ID: out.TurnID, Disposition: out.Disposition, CommandID: id, sub: sub, done: make(chan struct{}), stop: make(chan struct{}), events: make(chan Notification, size)}
	go t.follow(o.Stream)
	return t, nil
}

// Events contains events only when TurnOptions.Stream is true. Drain it while
// the turn is running. Item deltas are correlated using their item-start record.
func (t *Turn) Events() <-chan Notification { return t.events }
func (t *Turn) CloseTracking()              { t.stopOnce.Do(func() { close(t.stop) }) }
func (t *Turn) follow(stream bool) {
	defer close(t.done)
	defer close(t.events)
	defer t.sub.Close()
	items := make(map[string]bool)
	for {
		select {
		case <-t.stop:
			t.err = &Error{Kind: "transport", Message: "turn tracking stopped; native turn may still be running", Err: ErrClosed}
			return
		case n, ok := <-t.sub.Events():
			if !ok {
				t.err = &Error{Kind: "transport", Message: "turn outcome unknown", Err: t.sub.Err()}
				return
			}
			if n.Method == "view/gap" {
				t.err = &Error{Kind: "transport", Message: "turn view incomplete; recover durable history", Err: ErrGap, Data: n.Params}
				return
			}
			_, turnID, itemID := n.Identity()
			if turnID != "" && itemID != "" {
				items[itemID] = turnID == t.ID
			}
			if turnID != t.ID && (turnID != "" || itemID == "" || !items[itemID]) {
				continue
			}
			if stream {
				select {
				case t.events <- n.clone():
				default:
					t.err = &Error{Kind: "transport", Message: "turn stream overflow; native turn may still be running", Err: ErrOverflow}
					return
				}
			}
			switch n.Method {
			case "turn/completed":
				if err := json.Unmarshal(n.Params, &t.result); err != nil || t.result.Terminal == "" {
					t.err = &Error{Kind: "protocol", Message: "invalid turn terminal", Err: err}
					return
				}
				t.result.Raw = append(json.RawMessage(nil), n.Raw...)
				return
			case "turn/unqueued":
				t.result = TurnResult{SessionID: t.SessionID, TurnID: t.ID, Terminal: "unqueued", Raw: append(json.RawMessage(nil), n.Raw...), ViewCursor: n.Cursor()}
				return
			}
		}
	}
}

// Wait waits for a terminal observation. Cancellation only stops this wait.
// A nil error means the terminal is known; inspect Terminal for completed,
// failed, cancelled, unqueued, or a future value. It never implies success.
func (t *Turn) Wait(ctx context.Context) (TurnResult, error) {
	select {
	case <-t.done:
		return t.copyResult()
	default:
	}
	select {
	case <-t.done:
		return t.copyResult()
	case <-ctx.Done():
		return TurnResult{}, ctx.Err()
	}
}
func (t *Turn) copyResult() (TurnResult, error) {
	r := t.result
	r.Raw = append(json.RawMessage(nil), r.Raw...)
	r.Error = append(json.RawMessage(nil), r.Error...)
	r.Usage = append(json.RawMessage(nil), r.Usage...)
	return r, t.err
}
