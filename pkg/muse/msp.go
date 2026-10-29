package muse

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

const knownFingerprint = "sha256:c94c3d1f2f51e6424074b9ec43eac3eedcf532d3dcd77351ec1383a87d3049c6"

type rpcMessage struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	} `json:"error"`
}
type MSPHost struct {
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	stdout        io.ReadCloser
	writeMu       sync.Mutex
	mu            sync.Mutex
	pending       map[string]chan rpcMessage
	notifications chan Notification
	requests      chan Request
	done          chan struct{}
	processDone   chan struct{}
	next          atomic.Int64
	closeOnce     sync.Once
	failOnce      sync.Once
	err           error
	turnWaiters   map[string]chan TurnResult
	terminals     map[string]TurnResult
	Initialize    InitializeResult
	Warnings      []string
}

// StartMSP starts an owned host. ctx governs startup, not the returned host lifetime.
func (c *Client) StartMSP(ctx context.Context, o HostOptions) (*MSPHost, error) {
	args := []string{"serve"}
	for _, v := range []struct{ k, v string }{{"--provider", o.Provider}, {"--model", o.Model}, {"--preset", o.Preset}, {"--sandbox-network", o.SandboxNetwork}} {
		if v.v != "" {
			args = append(args, v.k, v.v)
		}
	}
	for _, v := range o.Config {
		args = append(args, "--config", v)
	}
	for _, v := range []struct {
		k  string
		on bool
	}{{"--no-session-log", o.NoSessionLog}, {"--trust-workspace", o.TrustWorkspace}, {"--disable-sandbox", o.DisableSandbox}, {"--disable-write", o.DisableWrite}, {"--disable-shell", o.DisableShell}} {
		if v.on {
			args = append(args, v.k)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cmd := exec.Command(c.BinPath, args...)
	cmd.Dir = c.WorkingDir
	if o.WorkingDir != "" {
		cmd.Dir = o.WorkingDir
	}
	cmd.Env = append(append(cmd.Environ(), c.Env...), o.Env...)
	cmd.Stderr = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("msp stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("msp stdout: %w", err)
	}
	if err = cmd.Start(); err != nil {
		return nil, &Error{Kind: "process", Message: "start MSP", Err: err}
	}
	h := &MSPHost{cmd: cmd, stdin: stdin, stdout: stdout, pending: make(map[string]chan rpcMessage), notifications: make(chan Notification, 512), requests: make(chan Request, 32), done: make(chan struct{}), processDone: make(chan struct{}), turnWaiters: make(map[string]chan TurnResult), terminals: make(map[string]TurnResult)}
	go h.readLoop()
	if o.ClientInfo.Name == "" {
		o.ClientInfo.Name = "muse_go_sdk"
	}
	if o.ClientInfo.Version == "" {
		o.ClientInfo.Version = "0.1.0"
	}
	params := map[string]any{"clientInfo": o.ClientInfo, "capabilities": map[string]any{"userInputDialogs": o.UserInputDialogs}}
	if err = h.Call(ctx, "initialize", params, &h.Initialize); err != nil {
		_ = h.Close(context.Background())
		return nil, fmt.Errorf("msp initialize: %w", err)
	}
	if h.Initialize.Schema.Version != 1 {
		_ = h.Close(context.Background())
		return nil, &Error{Kind: "compatibility", Message: fmt.Sprintf("unsupported MSP envelope version %d", h.Initialize.Schema.Version)}
	}
	if h.Initialize.Schema.Fingerprint != knownFingerprint {
		h.Warnings = append(h.Warnings, "MSP stable schema fingerprint differs from tested version")
	}
	if err = h.Notify("initialized", nil); err != nil {
		_ = h.Close(context.Background())
		return nil, err
	}
	return h, nil
}
func (h *MSPHost) PID() int                           { return h.cmd.Process.Pid }
func (h *MSPHost) Notifications() <-chan Notification { return h.notifications }
func (h *MSPHost) Requests() <-chan Request           { return h.requests }
func (h *MSPHost) Done() <-chan struct{}              { return h.done }
func (h *MSPHost) Err() error                         { h.mu.Lock(); defer h.mu.Unlock(); return h.err }
func (h *MSPHost) fail(err error) {
	h.failOnce.Do(func() { h.mu.Lock(); h.err = err; h.mu.Unlock(); close(h.done) })
}
func (h *MSPHost) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	h.writeMu.Lock()
	defer h.writeMu.Unlock()
	select {
	case <-h.done:
		return &Error{Kind: "transport", Message: "host closed", Err: h.Err()}
	default:
	}
	for len(b) > 0 {
		n, e := h.stdin.Write(b)
		if e != nil {
			h.fail(e)
			return &Error{Kind: "transport", Message: "write MSP", Err: e}
		}
		if n == 0 {
			h.fail(io.ErrShortWrite)
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}

// writeContext bounds a blocked pipe write. Cancellation while writing closes
// the host because the peer may have received a partial request.
func (h *MSPHost) writeContext(ctx context.Context, v any) error {
	result := make(chan error, 1)
	go func() { result <- h.write(v) }()
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		h.fail(&Error{Kind: "transport", Message: "write cancelled; request outcome may be unknown", Err: ctx.Err()})
		_ = h.cmd.Process.Kill()
		return ctx.Err()
	}
}
func (h *MSPHost) Notify(method string, params any) error {
	m := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		m["params"] = params
	}
	return h.write(m)
}
func (h *MSPHost) Call(ctx context.Context, method string, params any, result any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	id := h.next.Add(1)
	key := fmt.Sprint(id)
	ch := make(chan rpcMessage, 1)
	h.mu.Lock()
	h.pending[key] = ch
	h.mu.Unlock()
	defer func() { h.mu.Lock(); delete(h.pending, key); h.mu.Unlock() }()
	msg := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	if err := h.writeContext(ctx, msg); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-h.done:
		return &Error{Kind: "transport", Message: "host closed with request pending; outcome may be unknown", Err: h.Err()}
	case r := <-ch:
		if r.Error != nil {
			return &Error{Kind: "remote", Message: r.Error.Message, Code: r.Error.Code, Data: r.Error.Data}
		}
		if result != nil {
			if err := json.Unmarshal(r.Result, result); err != nil {
				return &Error{Kind: "protocol", Message: "decode response", Err: err}
			}
		}
		return nil
	}
}
func (h *MSPHost) readLoop() {
	defer close(h.processDone)
	defer close(h.notifications)
	defer close(h.requests)
	sc := bufio.NewScanner(h.stdout)
	sc.Buffer(make([]byte, 65536), 8<<20)
	for sc.Scan() {
		raw := append([]byte(nil), sc.Bytes()...)
		var m rpcMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			h.fail(&Error{Kind: "protocol", Message: "invalid MSP frame", Err: err})
			_ = h.cmd.Process.Kill()
			break
		}
		if m.Method == "" && len(m.ID) > 0 {
			key := string(m.ID)
			h.mu.Lock()
			ch := h.pending[key]
			h.mu.Unlock()
			if ch != nil {
				select {
				case ch <- m:
				default:
				}
			}
			continue
		}
		if m.Method != "" && len(m.ID) > 0 && string(m.ID) != "null" {
			select {
			case h.requests <- Request{ID: m.ID, Params: m.Params, Method: m.Method, host: h}:
			default:
				h.fail(errors.New("MSP request queue overflow"))
				_ = h.cmd.Process.Kill()
				goto end
			}
			continue
		}
		if m.Method != "" {
			if m.Method == "turn/completed" {
				var tr TurnResult
				if json.Unmarshal(m.Params, &tr) == nil {
					tr.Raw = raw
					h.mu.Lock()
					if ch := h.turnWaiters[tr.TurnID]; ch != nil {
						select {
						case ch <- tr:
						default:
						}
					} else if len(h.terminals) < 512 {
						h.terminals[tr.TurnID] = tr
					} else {
						h.mu.Unlock()
						h.fail(errors.New("MSP terminal queue overflow"))
						_ = h.cmd.Process.Kill()
						goto end
					}
					h.mu.Unlock()
				}
			}
			select {
			case h.notifications <- Notification{Method: m.Method, Params: m.Params, Raw: raw}:
			default:
				h.fail(errors.New("MSP notification queue overflow"))
				_ = h.cmd.Process.Kill()
				goto end
			}
		}
	}
end:
	err := sc.Err()
	if err != nil {
		h.fail(&Error{Kind: "transport", Message: "read MSP", Err: err})
	} else {
		h.fail(io.EOF)
	}
	_ = h.stdin.Close()
	_ = h.cmd.Wait()
}

// Close stops and reaps the process. A context without deadline uses a two-second grace period.
func (h *MSPHost) Close(ctx context.Context) error {
	h.closeOnce.Do(func() { _ = h.stdin.Close() })
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
	}
	select {
	case <-h.processDone:
		return nil
	case <-ctx.Done():
		_ = h.cmd.Process.Kill()
		<-h.processDone
		return ctx.Err()
	}
}
