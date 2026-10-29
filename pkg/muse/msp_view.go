package muse

import (
	"context"
	"encoding/json"
)

// ViewPage contains durable view events in server order. NextCursor is nil at
// the end. Raw preserves future fields. Ephemeral deltas cannot be replayed.
type ViewPage struct {
	Events     []Notification
	NextCursor *string
	Raw        json.RawMessage
}

func (s *Session) ReadViewPage(ctx context.Context, cursor string, limit int) (ViewPage, error) {
	raw, err := s.ViewPage(ctx, cursor, limit)
	if err != nil {
		return ViewPage{}, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(raw, &fields); err != nil || len(fields["events"]) == 0 || string(fields["events"]) == "null" || len(fields["nextCursor"]) == 0 {
		return ViewPage{}, &Error{Kind: "protocol", Message: "incomplete view page", Err: err}
	}
	var wire struct {
		Events     []json.RawMessage `json:"events"`
		NextCursor *string           `json:"nextCursor"`
	}
	if err = json.Unmarshal(raw, &wire); err != nil {
		return ViewPage{}, &Error{Kind: "protocol", Message: "decode view page", Err: err}
	}
	page := ViewPage{NextCursor: wire.NextCursor, Raw: raw}
	for _, event := range wire.Events {
		var n struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err = json.Unmarshal(event, &n); err != nil || n.Method == "" {
			return ViewPage{}, &Error{Kind: "protocol", Message: "invalid view event", Err: err}
		}
		page.Events = append(page.Events, Notification{Method: n.Method, Params: n.Params, Raw: event})
	}
	return page, nil
}

// ReplayView visits durable events strictly after an observed cursor, in page
// order, until the host reports the end. Use a deadline with an actively growing
// view. Replace items by ID/revision; never append completed text to old deltas.
// This does not merge live events. Subscribe first, buffer live events, replay,
// then reconcile by item revision and event identity in the application's view.
func (s *Session) ReplayView(ctx context.Context, after string, visit func(Notification) error) error {
	if visit == nil {
		return &Error{Kind: "validation", Message: "view visitor is required"}
	}
	seen := map[string]bool{after: true}
	for {
		page, err := s.ReadViewPage(ctx, after, 100)
		if err != nil {
			return err
		}
		for _, n := range page.Events {
			if err = ctx.Err(); err != nil {
				return err
			}
			if err = visit(n); err != nil {
				return err
			}
		}
		if page.NextCursor == nil {
			return nil
		}
		after = *page.NextCursor
		if after == "" || seen[after] {
			return &Error{Kind: "protocol", Message: "view pagination made no progress"}
		}
		seen[after] = true
	}
}
