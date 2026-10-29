package muse

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

func commandID(id string) (string, error) {
	if id != "" {
		u, err := uuid.Parse(id)
		if err != nil || u.Version() != 7 {
			return "", fmt.Errorf("commandId must be UUIDv7")
		}
		return id, nil
	}
	u, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

type Session struct {
	host   *MSPHost
	Info   SessionInfo
	Cursor string
}

func (h *MSPHost) StartSession(ctx context.Context, o SessionOptions) (*Session, error) {
	id, err := commandID(o.CommandID)
	if err != nil {
		return nil, err
	}
	o.CommandID = id
	var out struct {
		Session SessionInfo `json:"session"`
		Cursor  string      `json:"viewCursor"`
	}
	if err = h.Call(ctx, "session/start", o, &out); err != nil {
		return nil, err
	}
	if out.Session.SessionID == "" {
		return nil, &Error{Kind: "protocol", Message: "session/start returned no sessionId"}
	}
	return &Session{host: h, Info: out.Session, Cursor: out.Cursor}, nil
}
func (h *MSPHost) ResumeSession(ctx context.Context, sessionID string) (*Session, json.RawMessage, error) {
	if sessionID == "" {
		return nil, nil, &Error{Kind: "validation", Message: "session ID is required"}
	}
	id, err := commandID("")
	if err != nil {
		return nil, nil, err
	}
	var raw json.RawMessage
	err = h.Call(ctx, "session/resume", map[string]any{"sessionId": sessionID, "commandId": id}, &raw)
	if err != nil {
		return nil, nil, err
	}
	var out struct {
		Session SessionInfo `json:"session"`
		Cursor  string      `json:"viewCursor"`
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		return nil, nil, err
	}
	if out.Session.SessionID != sessionID {
		return nil, nil, &Error{Kind: "protocol", Message: "resume returned a different session"}
	}
	return &Session{host: h, Info: out.Session, Cursor: out.Cursor}, raw, nil
}

func (t *Turn) action(ctx context.Context, method string) error {
	id, err := commandID("")
	if err != nil {
		return err
	}
	return t.host.Call(ctx, method, map[string]any{"commandId": id, "sessionId": t.SessionID, "turnId": t.ID}, nil)
}
func (t *Turn) Interrupt(ctx context.Context) error { return t.action(ctx, "turn/interrupt") }
func (t *Turn) Cancel(ctx context.Context) error    { return t.action(ctx, "turn/cancel") }
func (s *Session) Command(ctx context.Context, method string, params map[string]any, result any) error {
	id, err := commandID("")
	if err != nil {
		return err
	}
	p := make(map[string]any, len(params)+2)
	for k, v := range params {
		p[k] = v
	}
	p["commandId"] = id
	p["sessionId"] = s.Info.SessionID
	return s.host.Call(ctx, method, p, result)
}

// Pending returns the host's current approval and user-input payloads.
func (s *Session) Pending(ctx context.Context) (json.RawMessage, error) {
	var out json.RawMessage
	err := s.host.Call(ctx, "approval/listPending", map[string]any{"sessionId": s.Info.SessionID}, &out)
	return out, err
}

// DecideApproval submits a server-offered choice for the current requirement.
func (s *Session) DecideApproval(ctx context.Context, approvalID, choiceID string, requirement json.RawMessage) error {
	if approvalID == "" || choiceID == "" || !json.Valid(requirement) || string(requirement) == "null" {
		return &Error{Kind: "validation", Message: "approval, choice and requirement are required"}
	}
	var ref struct {
		ApprovalID  string `json:"approvalId"`
		SourceIndex *int   `json:"sourceIndex"`
	}
	if err := json.Unmarshal(requirement, &ref); err != nil || ref.ApprovalID != approvalID || ref.SourceIndex == nil || *ref.SourceIndex < 0 {
		return &Error{Kind: "validation", Message: "requirement must identify the same approval and a nonnegative source index"}
	}
	return s.Command(ctx, "approval/decide", map[string]any{"approvalId": approvalID, "choiceId": choiceID, "requirementId": requirement}, nil)
}

// AnswerInput submits answers that use the question IDs from the request.
func (s *Session) AnswerInput(ctx context.Context, inputID string, answers []InputAnswer) error {
	if inputID == "" || len(answers) == 0 {
		return &Error{Kind: "validation", Message: "input ID and answers are required"}
	}
	seen := make(map[string]bool, len(answers))
	for _, answer := range answers {
		if answer.QuestionID == "" || seen[answer.QuestionID] {
			return &Error{Kind: "validation", Message: "answers require unique question IDs"}
		}
		seen[answer.QuestionID] = true
	}
	return s.Command(ctx, "userInput/answer", map[string]any{"userInputId": inputID, "answers": answers}, nil)
}
func (s *Session) SetApprovalMode(ctx context.Context, mode string) error {
	switch mode {
	case "allowAll", "promptUnmatched", "onRequest", "denyUnmatched":
	default:
		return &Error{Kind: "validation", Message: "unknown approval mode"}
	}
	return s.Command(ctx, "session/setApprovalMode", map[string]any{"mode": mode}, nil)
}

// Leave admits release of the session; the terminal result arrives asynchronously.
func (s *Session) Leave(ctx context.Context, stop bool) error {
	return s.Command(ctx, "session/leave", map[string]any{"stop": stop}, nil)
}

// ViewPage retrieves cursor-paged session events. Cursors are opaque.
func (s *Session) ViewPage(ctx context.Context, cursor string, limit int) (json.RawMessage, error) {
	if limit < 1 || limit > 1000 {
		return nil, &Error{Kind: "validation", Message: "view limit must be 1 through 1000"}
	}
	params := map[string]any{"sessionId": s.Info.SessionID, "limit": limit}
	if cursor != "" {
		params["cursor"] = cursor
	}
	var out json.RawMessage
	err := s.host.Call(ctx, "view/page", params, &out)
	return out, err
}

// CancelInput asks the host to cancel a pending user-input request.
func (s *Session) CancelInput(ctx context.Context, inputID string) error {
	if inputID == "" {
		return &Error{Kind: "validation", Message: "input ID is required"}
	}
	return s.Command(ctx, "userInput/cancel", map[string]any{"userInputId": inputID}, nil)
}

// SubscribeView reattaches to the session view after an opaque cursor.
func (s *Session) SubscribeView(ctx context.Context, after string) (json.RawMessage, error) {
	params := map[string]any{"sessionId": s.Info.SessionID}
	if after != "" {
		params["after"] = after
	}
	var out json.RawMessage
	err := s.host.Call(ctx, "view/subscribe", params, &out)
	return out, err
}
