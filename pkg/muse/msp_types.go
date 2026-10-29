package muse

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"
)

type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Title   string `json:"title,omitempty"`
}
type SchemaInfo struct {
	Version     int    `json:"version"`
	Fingerprint string `json:"fingerprint"`
}
type InitializeResult struct {
	ServerInfo          ClientInfo `json:"serverInfo"`
	Schema              SchemaInfo `json:"schema"`
	GrantedCapabilities []string   `json:"grantedCapabilities"`
	SessionDurability   string     `json:"sessionDurability"`
	SandboxAvailability string     `json:"sandboxAvailability"`
	ExperimentalAPI     bool       `json:"experimentalApi"`
}
type HostOptions struct {
	Provider, Model, Preset, WorkingDir, SandboxNetwork                      string
	Env, Config                                                              []string
	NoSessionLog, TrustWorkspace, DisableSandbox, DisableWrite, DisableShell bool
	UserInputDialogs                                                         bool
	ClientInfo                                                               ClientInfo
	Stderr                                                                   io.Writer
	DisableRequestUserInput                                                  bool
}

// Notification is a server event; Params and Raw preserve unknown fields.
type Notification struct {
	Method      string
	Params, Raw json.RawMessage
}

// Request is a server-initiated request; Respond acknowledges presentation only.
type Request struct {
	ID, Params json.RawMessage
	Method     string
	host       *MSPHost
	state      *receiptState
}

type receiptState struct {
	mu   sync.Mutex
	sent bool
}

// Respond sends a presentation receipt with a two-second write deadline.
func (r Request) Respond(err *Error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return r.RespondContext(ctx, err)
}

// RespondContext acknowledges presentation, never a permission choice. A request
// can be acknowledged only once, including across copies of Request.
func (r Request) RespondContext(ctx context.Context, responseErr *Error) error {
	if r.host == nil || r.state == nil {
		return &Error{Kind: "validation", Message: "request has no host"}
	}
	r.state.mu.Lock()
	defer r.state.mu.Unlock()
	if r.state.sent {
		return &Error{Kind: "validation", Message: "request already acknowledged"}
	}
	var frame any
	if responseErr != nil {
		frame = map[string]any{"jsonrpc": "2.0", "id": r.ID, "error": map[string]any{"code": responseErr.Code, "message": responseErr.Message, "data": responseErr.Data}}
	} else {
		frame = map[string]any{"jsonrpc": "2.0", "id": r.ID, "result": map[string]any{}}
	}
	if err := r.host.writeContext(ctx, frame); err != nil {
		return err
	}
	r.state.sent = true
	return nil
}

type SessionInfo struct {
	SessionID     string `json:"sessionId"`
	Status        string `json:"status"`
	WorkspaceRoot string `json:"workspaceRoot"`
}
type SessionOptions struct {
	CommandID         string   `json:"commandId,omitempty"`
	WorkspaceRoot     string   `json:"workspaceRoot,omitempty"`
	WorkspaceRoots    []string `json:"workspaceRoots,omitempty"`
	ModelID           string   `json:"modelId,omitempty"`
	ApprovalMode      string   `json:"approvalMode,omitempty"`
	PermissionProfile string   `json:"permissionProfile,omitempty"`
}
type InputPart struct {
	Type       string `json:"type"`
	Text       string `json:"text,omitempty"`
	MediaType  string `json:"mediaType,omitempty"`
	Base64Data string `json:"base64Data,omitempty"`
	Selector   string `json:"selector,omitempty"`
	Source     string `json:"source,omitempty"`
	Arguments  string `json:"arguments,omitempty"`
	UploadID   string `json:"uploadId,omitempty"`
	Width      *int   `json:"width,omitempty"`
	Height     *int   `json:"height,omitempty"`
}

func Text(s string) InputPart { return InputPart{Type: "text", Text: s} }

type TurnResult struct {
	SessionID  string          `json:"sessionId"`
	TurnID     string          `json:"turnId"`
	Terminal   string          `json:"terminal"`
	Error      json.RawMessage `json:"error,omitempty"`
	Raw        json.RawMessage `json:"-"`
	Usage      json.RawMessage `json:"usage,omitempty"`
	Reason     string          `json:"reason,omitempty"`
	ViewCursor string          `json:"viewCursor,omitempty"`
}
