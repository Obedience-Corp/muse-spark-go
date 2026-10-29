// Package muse provides a Go client for the Muse CLI and its MSP session host.
package muse

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// SDKVersion is the module release reported by the default MSP client handshake.
const SDKVersion = "0.1.0"

// Client holds process configuration. Do not mutate it while calls are running.
type Client struct {
	BinPath    string
	WorkingDir string
	Env        []string
	// Stderr receives native diagnostics. It may contain prompt or workspace data.
	// A writer shared by concurrent calls must be concurrency-safe.
	Stderr io.Writer
}

func NewClient(path string) *Client { return &Client{BinPath: path} }
func NewClientFromPath() (*Client, error) {
	p, err := exec.LookPath("muse")
	if err != nil {
		return nil, fmt.Errorf("locate muse: %w", err)
	}
	return NewClient(p), nil
}
func (c *Client) command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, c.BinPath, args...)
	cmd.Dir = c.WorkingDir
	cmd.Env = append(cmd.Environ(), c.Env...)
	cmd.Stderr = c.Stderr
	cmd.WaitDelay = time.Second
	return cmd
}
func (c *Client) Version(ctx context.Context) (string, error) {
	b, err := c.command(ctx, "--version").Output()
	if err != nil {
		return "", fmt.Errorf("muse version: %w", errors.Join(err, contextError(ctx)))
	}
	return string(b), nil
}

// RunPrompt runs a prompt without a deadline. Prefer RunPromptCtx in services.
func (c *Client) RunPrompt(prompt string, opts *RunOptions) (*Result, error) {
	return c.RunPromptCtx(context.Background(), prompt, opts)
}
