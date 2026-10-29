package muse

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const knownFingerprint = "sha256:c94c3d1f2f51e6424074b9ec43eac3eedcf532d3dcd77351ec1383a87d3049c6"
const maxFrameBytes = 8 << 20

var ErrClosed = errors.New("MSP host closed")
var ErrOverflow = errors.New("MSP event queue overflow")
var ErrGap = errors.New("MSP view has a delivery gap")

type rpcMessage struct {
	Version string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	} `json:"error"`
}

// MSPHost owns one Muse process. Call Close to release it. Public handshake
// metadata is immutable; calls and subscriptions may be used concurrently.
type MSPHost struct {
	cmd                           *exec.Cmd
	stdin                         io.WriteCloser
	stdout                        io.ReadCloser
	writer                        chan struct{}
	mu                            sync.Mutex
	pending                       map[string]chan rpcMessage
	subscribers                   map[*Subscription]struct{}
	notifications                 *Subscription
	requests                      chan Request
	done, processDone, readerDone chan struct{}
	processExited                 chan struct{}
	processErr                    error
	next                          atomic.Int64
	closeOnce, failOnce           sync.Once
	err                           error
	Initialize                    InitializeResult
	Warnings                      []string
}

// StartMSP uses ctx for startup only. The returned host lives until Close or failure.
func (c *Client) StartMSP(ctx context.Context, o HostOptions) (*MSPHost, error) {
	args, err := o.args()
	if err != nil {
		return nil, &Error{Kind: "validation", Message: "host options", Err: err}
	}
	if err = contextError(ctx); err != nil {
		return nil, err
	}
	cmd := exec.Command(c.BinPath, args...)
	cmd.Dir = c.WorkingDir
	if o.WorkingDir != "" {
		cmd.Dir = o.WorkingDir
	}
	cmd.Env = append(append(cmd.Environ(), c.Env...), o.Env...)
	cmd.Stderr = c.Stderr
	if o.Stderr != nil {
		cmd.Stderr = o.Stderr
	} // nil discards diagnostics; never merges them into protocol stdout.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("msp stdin: %w", err)
	}
	// An owned pipe lets the reaper run independently of the reader, including
	// when a child exits with a descendant still holding its stdout descriptor.
	stdout, childOut, err := os.Pipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("msp stdout: %w", err)
	}
	cmd.Stdout = childOut
	cmd.WaitDelay = time.Second
	if err = cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = childOut.Close()
		return nil, &Error{Kind: "process", Message: "start MSP", Err: err}
	}
	_ = childOut.Close()
	h := &MSPHost{cmd: cmd, stdin: stdin, stdout: stdout, writer: make(chan struct{}, 1), pending: make(map[string]chan rpcMessage), subscribers: make(map[*Subscription]struct{}), requests: make(chan Request, 32), done: make(chan struct{}), processDone: make(chan struct{}), readerDone: make(chan struct{}), processExited: make(chan struct{})}
	go h.readLoop()
	go func() {
		err := cmd.Wait()
		if err != nil {
			h.processErr = &Error{Kind: "process", Message: "MSP process exited", ExitCode: cmd.ProcessState.ExitCode(), Err: err}
		}
		close(h.processExited)
		// Allow the reader to consume buffered terminal/response frames first.
		select {
		case <-h.readerDone:
		case <-time.After(100 * time.Millisecond):
			_ = stdout.Close()
			<-h.readerDone
		}
		if err != nil {
			h.fail(&Error{Kind: "process", Message: "MSP process exited", ExitCode: cmd.ProcessState.ExitCode(), Err: err})
		} else {
			h.fail(io.EOF)
		}
		_ = stdout.Close()
		close(h.processDone)
	}()
	if o.ClientInfo.Name == "" {
		o.ClientInfo.Name = "muse_go_sdk"
	}
	if o.ClientInfo.Version == "" {
		o.ClientInfo.Version = SDKVersion
	}
	params := map[string]any{"clientInfo": o.ClientInfo, "capabilities": map[string]any{"userInputDialogs": o.UserInputDialogs}}
	if err = h.Call(ctx, "initialize", params, &h.Initialize); err != nil {
		h.abortStartup(err)
		return nil, fmt.Errorf("msp initialize: %w", err)
	}
	if h.Initialize.Schema.Version != 1 {
		err = &Error{Kind: "compatibility", Message: fmt.Sprintf("unsupported MSP schema version %d", h.Initialize.Schema.Version)}
		h.abortStartup(err)
		return nil, err
	}
	if h.Initialize.Schema.Fingerprint != knownFingerprint {
		h.Warnings = append(h.Warnings, "MSP stable schema fingerprint differs from tested version")
	}
	if err = h.NotifyContext(ctx, "initialized", nil); err != nil {
		h.abortStartup(err)
		return nil, err
	}
	return h, nil
}
func (h *MSPHost) PID() int                 { return h.cmd.Process.Pid }
func (h *MSPHost) Requests() <-chan Request { return h.requests }
func (h *MSPHost) Done() <-chan struct{}    { return h.done }
func (h *MSPHost) Err() error               { h.mu.Lock(); defer h.mu.Unlock(); return h.err }
func (h *MSPHost) fail(err error) {
	h.failOnce.Do(func() {
		h.mu.Lock()
		h.err = err
		for s := range h.subscribers {
			s.finish(err)
			delete(h.subscribers, s)
		}
		h.mu.Unlock()
		close(h.done)
		_ = h.stdin.Close()
		_ = h.cmd.Process.Kill()
	})
}

// writeContext serializes complete frames. A timeout waiting for the writer is
// harmless; a timeout during a write terminates the host (partial-frame ambiguity).
func (h *MSPHost) writeContext(ctx context.Context, v any) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return &Error{Kind: "validation", Message: "encode MSP frame", Err: err}
	}
	if len(b)+1 > maxFrameBytes {
		return &Error{Kind: "validation", Message: "MSP frame exceeds 8 MiB"}
	}
	select {
	case h.writer <- struct{}{}:
	case <-ctx.Done():
		return contextError(ctx)
	case <-h.done:
		return h.Err()
	}
	defer func() { <-h.writer }()
	if err := contextError(ctx); err != nil {
		return err
	}
	select {
	case <-h.done:
		return h.Err()
	default:
	}
	result := make(chan error, 1)
	go func() {
		b = append(b, '\n')
		for len(b) > 0 {
			n, e := h.stdin.Write(b)
			if e != nil {
				result <- e
				return
			}
			if n == 0 {
				result <- io.ErrShortWrite
				return
			}
			b = b[n:]
		}
		result <- nil
	}()
	select {
	case err = <-result:
		if err != nil {
			err = &Error{Kind: "transport", Message: "write MSP; outcome may be unknown", Err: err}
			h.fail(err)
		}
		return err
	case <-ctx.Done():
		err = &Error{Kind: "transport", Message: "write cancelled; outcome may be unknown", Err: contextError(ctx)}
		h.fail(err)
		<-result
		return err
	case <-h.done:
		<-result
		return h.Err()
	}
}

// Notify uses a bounded default write deadline. Prefer NotifyContext for explicit control.
func (h *MSPHost) Notify(method string, params any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return h.NotifyContext(ctx, method, params)
}
func (h *MSPHost) NotifyContext(ctx context.Context, method string, params any) error {
	if method == "" {
		return &Error{Kind: "validation", Message: "empty method"}
	}
	m := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		m["params"] = params
	}
	return h.writeContext(ctx, m)
}
func (h *MSPHost) Call(ctx context.Context, method string, params any, result any) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if method == "" {
		return &Error{Kind: "validation", Message: "empty method"}
	}
	id := h.next.Add(1)
	key := strconv.FormatInt(id, 10)
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
	var r rpcMessage
	select {
	case r = <-ch:
	case <-ctx.Done():
		return contextError(ctx)
	case <-h.done:
		// A received reply is authoritative even if EOF immediately follows it.
		select {
		case r = <-ch:
		default:
			return &Error{Kind: "transport", Message: "host closed with request pending; outcome may be unknown", Err: h.Err()}
		}
	}
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
func (h *MSPHost) readLoop() {
	defer close(h.readerDone)
	defer close(h.requests)
	sc := bufio.NewScanner(h.stdout)
	sc.Buffer(make([]byte, 65536), maxFrameBytes)
	for sc.Scan() {
		raw := append(json.RawMessage(nil), sc.Bytes()...)
		var m rpcMessage
		err := json.Unmarshal(raw, &m)
		hasID := len(m.ID) > 0 && string(m.ID) != "null"
		if err != nil || m.Version != "2.0" || (m.Method == "" && (!hasID || (m.Error == nil) == (len(m.Result) == 0))) || (m.Method != "" && (m.Error != nil || len(m.Result) > 0)) {
			h.fail(&Error{Kind: "protocol", Message: "invalid MSP envelope", Err: err})
			return
		}
		if hasID && !validRPCID(m.ID) {
			h.fail(&Error{Kind: "protocol", Message: "invalid MSP request ID"})
			return
		}
		if m.Method == "" {
			h.mu.Lock()
			ch := h.pending[string(m.ID)]
			h.mu.Unlock()
			if ch != nil {
				select {
				case ch <- m:
				default:
				}
			}
			continue
		}
		if hasID {
			req := Request{ID: m.ID, Params: m.Params, Method: m.Method, host: h, state: &receiptState{gate: make(chan struct{}, 1)}}
			select {
			case h.requests <- req:
			default:
				h.fail(&Error{Kind: "transport", Message: "request queue overflow", Err: ErrOverflow})
				return
			}
			continue
		}
		h.publish(Notification{Method: m.Method, Params: m.Params, Raw: raw})
	}
	if err := sc.Err(); err != nil {
		select {
		case <-h.processExited:
			if h.processErr != nil {
				h.fail(h.processErr)
				return
			}
		default:
		}
		h.fail(&Error{Kind: "protocol", Message: "read MSP frame", Err: err})
		return
	}
	// EOF normally races process exit. Preserve its exit status when available;
	// stdout closed by a still-running peer is a transport failure, not a hang.
	select {
	case <-h.processExited:
		if h.processErr != nil {
			h.fail(h.processErr)
		} else {
			h.fail(io.EOF)
		}
	case <-time.After(100 * time.Millisecond):
		h.fail(&Error{Kind: "transport", Message: "MSP stdout closed before process exit", Err: io.EOF})
	}

}
func validRPCID(id json.RawMessage) bool {
	var s string
	if json.Unmarshal(id, &s) == nil {
		return true
	}
	var n json.Number
	if json.Unmarshal(id, &n) != nil {
		return false
	}
	_, err := strconv.ParseInt(n.String(), 10, 64)
	return err == nil
}

// Close closes stdin, allows a graceful exit, then terminates and reaps the child.
// No deadline means a two-second grace period. It is safe to call concurrently.
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
	default:
	}
	select {
	case <-h.processDone:
		return nil
	case <-ctx.Done():
		h.fail(&Error{Kind: "transport", Message: "host close cancelled", Err: contextError(ctx)})
		<-h.processDone
		return contextError(ctx)
	}
}

// No host escapes a failed handshake. Abort immediately instead of spending a
// fresh background grace period after the caller has cancelled; still reap it.
func (h *MSPHost) abortStartup(err error) {
	h.fail(err)
	<-h.processDone
}
