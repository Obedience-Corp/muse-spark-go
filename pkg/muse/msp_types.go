package muse

import "encoding/json"

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
}

func (r Request) Respond(err *Error) error {
	if err != nil {
		return r.host.write(map[string]any{"jsonrpc": "2.0", "id": r.ID, "error": map[string]any{"code": err.Code, "message": err.Message}})
	}
	return r.host.write(map[string]any{"jsonrpc": "2.0", "id": r.ID, "result": map[string]any{}})
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
}

func Text(s string) InputPart { return InputPart{Type: "text", Text: s} }

type TurnResult struct {
	SessionID string          `json:"sessionId"`
	TurnID    string          `json:"turnId"`
	Terminal  string          `json:"terminal"`
	Error     json.RawMessage `json:"error,omitempty"`
	Raw       json.RawMessage `json:"-"`
}
