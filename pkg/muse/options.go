package muse

import "fmt"

// RunOptions configures one headless exec. Zero values leave Muse defaults intact.
type RunOptions struct {
	Provider, Model, Preset, ReasoningEffort, Workspace, ApprovalMode, PermissionProfile string
	Config                                                                               []string
	Images                                                                               []string
	NoSessionLog, NoForeignPersonalContext, DisableRequestUserInput, TrustWorkspace      bool
	DisableSandbox, DisableWrite, DisableShell                                           bool
	SandboxNetwork                                                                       string
}

func (o RunOptions) args() ([]string, error) {
	a := []string{"exec", "--json"}
	for _, v := range []struct{ k, v string }{{"--provider", o.Provider}, {"--model", o.Model}, {"--preset", o.Preset}, {"--reasoning-effort", o.ReasoningEffort}, {"--workspace", o.Workspace}, {"--approval-mode", o.ApprovalMode}, {"--permission-profile", o.PermissionProfile}, {"--sandbox-network", o.SandboxNetwork}} {
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
	}{{"--no-session-log", o.NoSessionLog}, {"--no-foreign-personal-context", o.NoForeignPersonalContext}, {"--disable-request-user-input", o.DisableRequestUserInput}, {"--trust-workspace", o.TrustWorkspace}, {"--disable-sandbox", o.DisableSandbox}, {"--disable-write", o.DisableWrite}, {"--disable-shell", o.DisableShell}} {
		if v.on {
			a = append(a, v.k)
		}
	}
	return a, nil
}
