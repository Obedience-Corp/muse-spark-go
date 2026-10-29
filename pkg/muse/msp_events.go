package muse

import (
	"encoding/json"
	"sync"
)

// Subscription is an independent, bounded event stream. Close it when finished.
// Overflow closes only this subscription; Err reports the loss. Buffered events
// remain readable after closure. Register before starting commands to see early events.
type Subscription struct {
	host      *MSPHost
	sessionID string
	mu        sync.Mutex
	events    chan Notification
	err       error
	closed    bool
}

func (s *Subscription) Events() <-chan Notification { return s.events }
func (s *Subscription) Err() error                  { s.mu.Lock(); defer s.mu.Unlock(); return s.err }
func (s *Subscription) finish(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		s.err = err
		close(s.events)
	}
}
func (s *Subscription) Close() {
	s.host.mu.Lock()
	defer s.host.mu.Unlock()
	delete(s.host.subscribers, s)
	s.finish(nil)
}
func (h *MSPHost) subscribeLocked(sessionID string, capacity int) *Subscription {
	s := &Subscription{host: h, sessionID: sessionID, events: make(chan Notification, capacity)}
	if h.err != nil {
		s.finish(h.err)
	} else {
		h.subscribers[s] = struct{}{}
	}
	return s
}

// Subscribe follows all events (empty sessionID), or one session. Capacity must
// be positive; zero selects 256. It does not issue a remote view/subscribe call.
func (h *MSPHost) Subscribe(sessionID string, capacity int) *Subscription {
	if capacity <= 0 {
		capacity = 256
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.subscribeLocked(sessionID, capacity)
}

// Notifications is a legacy single-consumer stream, activated on first call.
// Subscribe is preferred: it exposes overflow independently from host failure.
func (h *MSPHost) Notifications() <-chan Notification {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.notifications == nil {
		h.notifications = h.subscribeLocked("", 512)
	}
	return h.notifications.events
}
func (h *MSPHost) publish(n Notification) {
	sessionID, _, _ := n.Identity()
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subscribers {
		if s.sessionID != "" && s.sessionID != sessionID {
			continue
		}
		select {
		case s.events <- n.clone():
		default:
			s.finish(&Error{Kind: "transport", Message: "subscription overflow; resubscribe from last durable cursor", Err: ErrOverflow})
			delete(h.subscribers, s)
		}
	}
}

// Identity extracts session, turn and item IDs without interpreting opaque IDs.
// Deltas carry only an item ID; correlate them with item/started or item/completed.
func (n Notification) Identity() (sessionID, turnID, itemID string) {
	var p struct {
		SessionID string `json:"sessionId"`
		TurnID    string `json:"turnId"`
		ItemID    string `json:"itemId"`
		Session   struct {
			ID string `json:"sessionId"`
		} `json:"session"`
		Item struct {
			ID     string `json:"itemId"`
			TurnID string `json:"turnId"`
		} `json:"item"`
	}
	_ = json.Unmarshal(n.Params, &p)
	if p.SessionID == "" {
		p.SessionID = p.Session.ID
	}
	if p.TurnID == "" {
		p.TurnID = p.Item.TurnID
	}
	if p.ItemID == "" {
		p.ItemID = p.Item.ID
	}
	return p.SessionID, p.TurnID, p.ItemID
}

// Cursor returns an observed view cursor, or empty for non-view events.
func (n Notification) Cursor() string {
	var p struct {
		Cursor string `json:"viewCursor"`
	}
	_ = json.Unmarshal(n.Params, &p)
	return p.Cursor
}

func (n Notification) clone() Notification {
	n.Params = append(json.RawMessage(nil), n.Params...)
	n.Raw = append(json.RawMessage(nil), n.Raw...)
	return n
}
