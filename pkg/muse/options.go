package muse

import (
	"fmt"
	"io"
	"strconv"
)

// RunOptions configures one headless exec. Zero values leave Muse defaults intact.
type RunOptions struct {
	Provider, Model, Preset, ReasoningEffort, Workspace, ApprovalMode, PermissionProfile string
	Config                                                                               []string
	Images                                                                               []string
	NoSessionLog, NoForeignPersonalContext, DisableRequestUserInput, TrustWorkspace      bool
	DisableSandbox, DisableWrite, DisableShell                                           bool
	SandboxNetwork                                                                       string
	BaseURL, SessionID, OutputSchema, PromptFile                                         string
	Worktree, WorktreeBase, WorktreeExisting                                             string
	ContributorMode, ApprovalJudge                                                       string
	MaxModelSteps                                                                        *int
	MaxToolOutputBytes                                                                   *int
	ParallelToolCalls                                                                    *bool
	DisableWebTools, DisableReminders, AllowWorkspaceSwitch, UserInputAutoResolve        bool
	// APIKey is read from stdin by Muse, never placed in arguments. The caller
	// owns it and must ensure reads terminate; prefer a finite strings.Reader.
	APIKey io.Reader
}

func (o RunOptions) args() ([]string, error) {
	a := []string{"exec", "--json"}
	if o.DisableRequestUserInput && o.UserInputAutoResolve {
		return nil, fmt.Errorf("disabled user input conflicts with auto-resolve")
	}
	if o.WorktreeBase != "" && o.Worktree != "create" {
		return nil, fmt.Errorf("worktree base requires create mode")
	}
	if o.WorktreeExisting != "" && o.Worktree != "existing" {
		return nil, fmt.Errorf("worktree path requires existing mode")
	}
	if o.Worktree == "existing" && o.WorktreeExisting == "" {
		return nil, fmt.Errorf("existing worktree requires a path")
	}
	switch o.Worktree {
	case "", "off", "create", "existing":
	default:
		return nil, fmt.Errorf("invalid worktree mode")
	}
	if o.MaxModelSteps != nil {
		if *o.MaxModelSteps < 1 {
			return nil, fmt.Errorf("model step limit must be positive")
		}
		a = append(a, "--max-model-steps", strconv.Itoa(*o.MaxModelSteps))
	}
	if o.MaxToolOutputBytes != nil {
		v := *o.MaxToolOutputBytes
		if v < 0 || v > 0 && v < 9 {
			return nil, fmt.Errorf("tool output limit must be zero or at least nine")
		}
		a = append(a, "--max-tool-output-bytes", strconv.Itoa(v))
	}
	if o.ParallelToolCalls != nil {
		flag := "--no-parallel-tool-calls"
		if *o.ParallelToolCalls {
			flag = "--parallel-tool-calls"
		}
		a = append(a, flag)
	}
	if o.APIKey != nil {
		a = append(a, "--api-key-stdin")
	}

	for _, v := range []struct{ k, v string }{{"--provider", o.Provider}, {"--model", o.Model}, {"--preset", o.Preset}, {"--reasoning-effort", o.ReasoningEffort}, {"--workspace", o.Workspace}, {"--approval-mode", o.ApprovalMode}, {"--permission-profile", o.PermissionProfile}, {"--sandbox-network", o.SandboxNetwork}, {"--base-url", o.BaseURL}, {"--session-id", o.SessionID}, {"--output-schema", o.OutputSchema}, {"--prompt-file", o.PromptFile}, {"--worktree", o.Worktree}, {"--worktree-base", o.WorktreeBase}, {"--worktree-existing", o.WorktreeExisting}, {"--contributor-mode", o.ContributorMode}, {"--approval-judge", o.ApprovalJudge}} {
		if v.v != "" {
			a = append(a, v.k, v.v)
		}
	}
	for _, v := range o.Config {
		if v == "" {
			return nil, fmt.Errorf("empty config override")
		}
		a = append(a, "--config", v)
	}
	for _, v := range o.Images {
		if v == "" {
			return nil, fmt.Errorf("empty image path")
		}
		a = append(a, "--image", v)
	}
	for _, v := range []struct {
		k  string
		on bool
	}{{"--no-session-log", o.NoSessionLog}, {"--no-foreign-personal-context", o.NoForeignPersonalContext}, {"--disable-request-user-input", o.DisableRequestUserInput}, {"--trust-workspace", o.TrustWorkspace}, {"--disable-sandbox", o.DisableSandbox}, {"--disable-write", o.DisableWrite}, {"--disable-shell", o.DisableShell}, {"--disable-web-tools", o.DisableWebTools}, {"--disable-reminders", o.DisableReminders}, {"--allow-workspace-switch", o.AllowWorkspaceSwitch}, {"--user-input-auto-resolve", o.UserInputAutoResolve}} {
		if v.on {
			a = append(a, v.k)
		}
	}
	return a, nil
}

func (o HostOptions) args() ([]string, error) {
	a := []string{"serve"}
	if o.UserInputDialogs && o.DisableRequestUserInput {
		return nil, fmt.Errorf("user input dialogs conflict with disabled input tool")
	}
	for _, v := range []struct{ k, v string }{{"--provider", o.Provider}, {"--model", o.Model}, {"--preset", o.Preset}, {"--sandbox-network", o.SandboxNetwork}} {
		if v.v != "" {
			a = append(a, v.k, v.v)
		}
	}
	for _, v := range o.Config {
		if v == "" {
			return nil, fmt.Errorf("empty config override")
		}
		a = append(a, "--config", v)
	}
	for _, v := range []struct {
		k  string
		on bool
	}{{"--no-session-log", o.NoSessionLog}, {"--trust-workspace", o.TrustWorkspace}, {"--disable-sandbox", o.DisableSandbox}, {"--disable-write", o.DisableWrite}, {"--disable-shell", o.DisableShell}, {"--disable-request-user-input", o.DisableRequestUserInput}} {
		if v.on {
			a = append(a, v.k)
		}
	}
	return a, nil
}
