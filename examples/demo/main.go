// Command demo exercises the installed Muse binary with isolated echo sessions.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/Obedience-Corp/muse-spark-go/pkg/muse"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "demo:", err)
		os.Exit(1)
	}
}
func run() error {
	name := flag.String("demo", "streaming", "basic, streaming, sessions, resume, cancel, permissions, errors, or all")
	binary := flag.String("muse", "muse", "Muse executable path")
	prompt := flag.String("prompt", "Hello from the Muse Go SDK.", "echo prompt")
	interactive := flag.Bool("interactive", false, "read prompts until /quit (sessions demo)")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	// This is a real demo fixture, removed on return. Durable resume is checked
	// within it; it never opens the user's existing session or auth directory.
	dir, err := os.MkdirTemp("", "muse-sdk-demo-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	c := muse.NewClient(*binary)
	c.WorkingDir = dir
	c.Env = []string{"XDG_CONFIG_HOME=" + filepath.Join(dir, "config"), "XDG_CACHE_HOME=" + filepath.Join(dir, "cache"), "XDG_DATA_HOME=" + filepath.Join(dir, "data"), "MUSE_NO_AUTO_UPDATE=1"}
	version, err := c.Version(ctx)
	if err != nil {
		return err
	}
	fmt.Println(strings.TrimSpace(version))
	fmt.Println("Provider: echo | isolated configuration | no model credentials")
	names := []string{*name}
	if *name == "all" {
		names = []string{"basic", "streaming", "sessions", "resume", "cancel", "permissions", "errors"}
	}
	for _, n := range names {
		fmt.Printf("\n── %s ──\n", n)
		if err = demo(ctx, c, dir, n, *prompt, *interactive); err != nil {
			return fmt.Errorf("%s: %w", n, err)
		}
	}
	return nil
}
func execOptions() *muse.RunOptions {
	return &muse.RunOptions{Provider: "echo", Preset: "empty", NoSessionLog: true, NoForeignPersonalContext: true, DisableRequestUserInput: true, DisableReminders: true}
}
func host(ctx context.Context, c *muse.Client, durable bool) (*muse.MSPHost, error) {
	return c.StartMSP(ctx, muse.HostOptions{Provider: "echo", Preset: "empty", NoSessionLog: !durable, DisableRequestUserInput: true})
}
func demo(ctx context.Context, c *muse.Client, dir, name, prompt string, interactive bool) error {
	switch name {
	case "basic":
		r, err := c.RunPromptCtx(ctx, prompt, execOptions())
		if err != nil {
			return err
		}
		if r.Text != "echo: "+prompt {
			return fmt.Errorf("unexpected echo text %q", r.Text)
		}
		fmt.Printf("Response: %s\nTerminal: %s\n", r.Text, r.Terminal)
		return nil
	case "streaming":
		events, errs := c.StreamPrompt(ctx, prompt, execOptions())
		count := 0
		for event := range events {
			count++
			if strings.HasPrefix(event.Type, "run.terminal.") {
				var p struct {
					Text     string `json:"text"`
					Terminal string `json:"terminal"`
				}
				if err := json.Unmarshal(event.Payload, &p); err != nil {
					return err
				}
				fmt.Printf("%s\nTerminal: %s\n", p.Text, p.Terminal)
			} else {
				fmt.Println("event:", event.Type)
			}
		}
		if err := <-errs; err != nil {
			return err
		}
		fmt.Printf("Received %d runtime records\n", count)
		return nil
	case "errors":
		bad := muse.NewClient(filepath.Join(dir, "missing-muse"))
		_, err := bad.RunPromptCtx(ctx, prompt, execOptions())
		var native *muse.Error
		if !errors.As(err, &native) || native.Kind != "process" {
			return fmt.Errorf("expected typed process failure, got %v", err)
		}
		fmt.Println("Missing executable → process error")
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		_, err = c.RunPromptCtx(cancelled, prompt, execOptions())
		if !errors.Is(err, context.Canceled) {
			return fmt.Errorf("expected cancellation, got %v", err)
		}
		fmt.Println("Cancelled context → context.Canceled")
		return nil
	}
	durable := name == "resume"
	h, err := host(ctx, c, durable)
	if err != nil {
		return err
	}
	defer h.Close(context.Background())
	s, err := h.StartSession(ctx, muse.SessionOptions{WorkspaceRoot: dir})
	if err != nil {
		return err
	}
	switch name {
	case "sessions", "resume":
		if err = printTurn(ctx, s, prompt); err != nil {
			return err
		}
		if name == "resume" {
			id := s.Info.SessionID
			if err = h.Close(ctx); err != nil {
				return err
			}
			h2, e := host(ctx, c, true)
			if e != nil {
				return e
			}
			defer h2.Close(context.Background())
			s, _, err = h2.ResumeSession(ctx, id)
			if err != nil {
				return err
			}
			if s.Info.SessionID != id {
				return fmt.Errorf("session identity changed")
			}
			fmt.Println("Host restarted; native session ID preserved")
			count := 0
			if err = s.ReplayView(ctx, "", func(n muse.Notification) error { count++; return nil }); err != nil {
				return err
			}
			if count == 0 {
				return fmt.Errorf("durable history was empty")
			}
			fmt.Printf("Replayed %d durable view events\n", count)
		}
		if interactive && name == "sessions" {
			scanner := bufio.NewScanner(os.Stdin)
			for {
				fmt.Print("you> ")
				if !scanner.Scan() {
					return scanner.Err()
				}
				if scanner.Text() == "/quit" {
					break
				}
				if err = printTurn(ctx, s, scanner.Text()); err != nil {
					return err
				}
			}
			return nil
		}
		return printTurn(ctx, s, "A second turn in the same session.")
	case "cancel":
		t, e := s.Send(ctx, []muse.InputPart{muse.Text(strings.Repeat("cancellation demo ", 500))})
		if e != nil {
			return e
		}
		if err = t.Cancel(ctx); err != nil {
			return err
		}
		r, e := t.Wait(ctx)
		if e != nil {
			return e
		}
		fmt.Println("Cancellation terminal:", r.Terminal)
		if r.Terminal != "cancelled" {
			return fmt.Errorf("turn completed before cancellation could be proven: %s", r.Terminal)
		}
		t, e = s.Send(ctx, []muse.InputPart{muse.Text(strings.Repeat("interrupt demo ", 500))})
		if e != nil {
			return e
		}
		if err = t.Interrupt(ctx); err != nil {
			return err
		}
		r, e = t.Wait(ctx)
		if e != nil {
			return e
		}
		fmt.Println("Interruption terminal:", r.Terminal)
		if r.Terminal != "cancelled" {
			return fmt.Errorf("interrupt not proven: %s", r.Terminal)
		}
		return nil
	case "permissions":
		if err = s.SetApprovalMode(ctx, "denyUnmatched"); err != nil {
			return err
		}
		fmt.Println("Approval mode: denyUnmatched")
		pending, e := s.Pending(ctx)
		if e != nil {
			return e
		}
		fmt.Println("Pending requests:", string(pending))
		fmt.Println("Echo does not request tools; approval receipt/decision tests use the protocol fixture.")
		return nil
	default:
		return fmt.Errorf("unknown demo %q", name)
	}
}
func printTurn(ctx context.Context, s *muse.Session, prompt string) error {
	t, err := s.SendWithOptions(ctx, []muse.InputPart{muse.Text(prompt)}, muse.TurnOptions{Stream: true})
	if err != nil {
		return err
	}
	fmt.Print("muse> ")
	finalText := ""
	for n := range t.Events() {
		if n.Method == "item/completed" {
			var p struct {
				Item struct {
					Kind string `json:"kind"`
					Text string `json:"text"`
				} `json:"item"`
			}
			if err = json.Unmarshal(n.Params, &p); err != nil {
				return err
			}
			if p.Item.Kind == "agentMessage" {
				finalText = p.Item.Text
			}
		}
		if n.Method == "item/delta" {
			var p struct {
				Delta string `json:"delta"`
			}
			if err = json.Unmarshal(n.Params, &p); err != nil {
				return err
			}
			fmt.Print(p.Delta)
		}
	}
	r, err := t.Wait(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("\nTerminal: %s\n", r.Terminal)
	if finalText != "echo: "+prompt {
		return fmt.Errorf("unexpected authoritative echo text %q", finalText)
	}
	if r.Terminal != "completed" {
		return fmt.Errorf("unexpected terminal %s", r.Terminal)
	}
	return nil
}
