// Package muse provides a Go client for the Muse CLI and its MSP session host.
package muse

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// Client holds process configuration. Do not mutate it while calls are running.
type Client struct {
	BinPath    string
	WorkingDir string
	Env        []string
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
	cmd.Env = append(os.Environ(), c.Env...)
	return cmd
}
func (c *Client) Version(ctx context.Context) (string, error) {
	b, err := c.command(ctx, "--version").Output()
	if err != nil {
		return "", fmt.Errorf("muse version: %w", err)
	}
	return string(b), nil
}
