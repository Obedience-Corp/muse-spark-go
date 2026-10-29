package muse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}
func testSession(t *testing.T, h *MSPHost) *Session {
	t.Helper()
	s, err := h.StartSession(testContext(t), SessionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestRepeatedConcurrentWait(t *testing.T) {
	h := newFake(t)
	turn, err := testSession(t, h).Send(testContext(t), []InputPart{Text("hello")})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := turn.Wait(testContext(t))
			if err != nil || r.Terminal != "completed" {
				t.Errorf("%+v %v", r, err)
			}
			if len(r.Raw) > 0 {
				r.Raw[0] = 'x'
			}
		}()
	}
	wg.Wait()
}
func TestTurnEventsBeforeAckAndItemRouting(t *testing.T) {
	h := newFake(t)
	s := testSession(t, h)
	turn, err := s.SendWithOptions(testContext(t), []InputPart{Text("route")}, TurnOptions{Stream: true})
	if err != nil {
		t.Fatal(err)
	}
	var methods []string
	for n := range turn.Events() {
		methods = append(methods, n.Method)
		if strings.Contains(string(n.Params), "foreign") {
			t.Fatalf("cross-turn event: %s", n.Params)
		}
	}
	r, err := turn.Wait(testContext(t))
	if err != nil || r.Terminal != "completed" {
		t.Fatalf("%+v %v", r, err)
	}
	if strings.Join(methods, ",") != "item/started,item/delta,item/completed,turn/completed" {
		t.Fatal(methods)
	}
}
func TestUnqueuedIsTerminal(t *testing.T) {
	h := newFake(t)
	turn, err := testSession(t, h).Send(testContext(t), []InputPart{Text("unqueued")})
	if err != nil {
		t.Fatal(err)
	}
	r, err := turn.Wait(testContext(t))
	if err != nil || r.Terminal != "unqueued" {
		t.Fatalf("%+v %v", r, err)
	}
}
func TestWaitCancellationDoesNotCancelTurn(t *testing.T) {
	h := newFake(t)
	turn, err := testSession(t, h).Send(testContext(t), []InputPart{Text("hold")})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = turn.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err = turn.Cancel(testContext(t)); err != nil {
		t.Fatal(err)
	}
	r, err := turn.Wait(testContext(t))
	if err != nil || r.Terminal != "cancelled" {
		t.Fatalf("%+v %v", r, err)
	}
}
func TestGapIsExplicit(t *testing.T) {
	h := newFake(t)
	turn, err := testSession(t, h).Send(testContext(t), []InputPart{Text("gap")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = turn.Wait(testContext(t)); !errors.Is(err, ErrGap) {
		t.Fatal(err)
	}
}
func TestSlowSubscriberDoesNotKillHost(t *testing.T) {
	h := newFake(t)
	slow := h.Subscribe("s1", 1)
	defer slow.Close()
	fast := h.Subscribe("s1", 16)
	defer fast.Close()
	if err := h.Call(testContext(t), "burst", nil, nil); err != nil {
		t.Fatal(err)
	}
	for range slow.Events() {
	}
	if !errors.Is(slow.Err(), ErrOverflow) {
		t.Fatal(slow.Err())
	}
	if err := h.Call(testContext(t), "echo", nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(fast.events) != 4 {
		t.Fatalf("healthy subscriber got %d events", len(fast.events))
	}
}
func TestTurnStreamOverflow(t *testing.T) {
	h := newFake(t)
	turn, err := testSession(t, h).SendWithOptions(testContext(t), []InputPart{Text("route")}, TurnOptions{Stream: true, EventBuffer: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = turn.Wait(testContext(t)); !errors.Is(err, ErrOverflow) {
		t.Fatal(err)
	}
	if err = h.Call(testContext(t), "echo", nil, nil); err != nil {
		t.Fatal(err)
	}
}
func TestCloseFailsPendingAndReaps(t *testing.T) {
	h := newFake(t)
	result := make(chan error, 1)
	go func() { result <- h.Call(context.Background(), "no-response", nil, nil) }()
	if err := h.Close(testContext(t)); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("pending call succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("pending call hung")
	}
	select {
	case <-h.processDone:
	default:
		t.Fatal("child not reaped")
	}
	if err := h.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func TestMalformedMSPFrames(t *testing.T) {
	for _, method := range []string{"invalid-json", "invalid-version", "invalid-response", "invalid-id", "oversized"} {
		t.Run(method, func(t *testing.T) {
			h := newFake(t)
			err := h.Call(testContext(t), method, nil, nil)
			if err == nil {
				t.Fatal("accepted malformed frame")
			}
			select {
			case <-h.done:
			case <-time.After(time.Second):
				t.Fatal("host left alive")
			}
		})
	}
}
func TestCancelledWaitingWriterPreservesHost(t *testing.T) {
	h := newFake(t)
	h.writer <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := h.NotifyContext(ctx, "ignored", nil)
	<-h.writer
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err = h.Call(testContext(t), "echo", nil, nil); err != nil {
		t.Fatal(err)
	}
}
func TestReceiptCannotBeSentTwice(t *testing.T) {
	h := newFake(t)
	if err := h.Call(testContext(t), "question", nil, nil); err != nil {
		t.Fatal(err)
	}
	r := <-h.Requests()
	if err := r.Respond(nil); err != nil {
		t.Fatal(err)
	}
	if err := r.Respond(nil); err == nil {
		t.Fatal("duplicate receipt allowed")
	}
}
func TestViewReplay(t *testing.T) {
	h := newFake(t)
	s := testSession(t, h)
	var seen []string
	err := s.ReplayView(testContext(t), "opaque/start", func(n Notification) error { seen = append(seen, n.Cursor()); return nil })
	if err != nil || strings.Join(seen, ",") != "opaque/one,opaque/two" {
		t.Fatalf("%v %v", seen, err)
	}
	if err = s.ReplayView(testContext(t), "loop", func(Notification) error { return nil }); err == nil {
		t.Fatal("cyclic pagination accepted")
	}
	sentinel := errors.New("stop")
	if err = s.ReplayView(testContext(t), "opaque/start", func(Notification) error { return sentinel }); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
}
func TestNoGlobalDrainRequired(t *testing.T) {
	h := newFake(t)
	for i := 0; i < 160; i++ {
		if err := h.Call(testContext(t), "burst", nil, nil); err != nil {
			t.Fatal(err)
		}
	}
}
func TestExecNegativeCases(t *testing.T) {
	for _, scenario := range []string{"null", "no-type", "oversized", "duplicate", "bad-terminal", "exit"} {
		t.Run(scenario, func(t *testing.T) {
			c := NewClient(os.Args[0])
			c.Env = []string{"MUSE_FAKE_HOST=1", "MUSE_FAKE_SCENARIO=" + scenario}
			_, err := c.RunPromptCtx(testContext(t), "hello", nil)
			if err == nil {
				t.Fatal("expected failure")
			}
			if scenario == "exit" {
				var e *Error
				if !errors.As(err, &e) || e.ExitCode != 7 {
					t.Fatal(err)
				}
			}
		})
	}
}
func TestExecCancelledUndrainedStream(t *testing.T) {
	c := NewClient(os.Args[0])
	c.Env = []string{"MUSE_FAKE_HOST=1", "MUSE_FAKE_SCENARIO=flood"}
	ctx, cancel := context.WithCancel(context.Background())
	events, errs := c.StreamPrompt(ctx, "hello", nil)
	<-events
	cancel()
	for range events {
	}
	if err := <-errs; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestSchemaNegotiation(t *testing.T) {
	for _, scenario := range []string{"version", "fingerprint", "startup-hang"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
			defer cancel()
			h, err := NewClient(os.Args[0]).StartMSP(ctx, HostOptions{Env: []string{"MUSE_FAKE_HOST=1", "MUSE_FAKE_SCENARIO=" + scenario}})
			if scenario == "fingerprint" {
				if err != nil {
					t.Fatal(err)
				}
				defer h.Close(context.Background())
				if len(h.Warnings) != 1 {
					t.Fatal(h.Warnings)
				}
			} else if err == nil {
				h.Close(context.Background())
				t.Fatal("expected startup error")
			}
		})
	}
}

// Extra fake behaviors run in the child test executable; no filesystem fixtures.
func fakeHardening(method string, id, params json.RawMessage) bool {
	reply := func(result string) { fmt.Printf(`{"jsonrpc":"2.0","id":%s,"result":%s}`+"\n", id, result) }
	event := func(method, params string) {
		fmt.Printf(`{"jsonrpc":"2.0","method":%q,"params":%s}`+"\n", method, params)
	}
	switch method {
	case "two-sessions":
		event("unknown/new", `{"sessionId":"s1"}`)
		event("unknown/new", `{"sessionId":"s2"}`)
		reply(`{}`)
	case "reorder":
		var p struct {
			Value string `json:"value"`
		}
		_ = json.Unmarshal(params, &p)
		if heldResponseID == nil {
			heldResponseID = append(json.RawMessage(nil), id...)
			heldResponseValue = p.Value
		} else {
			reply(fmt.Sprintf(`{"value":%q}`, p.Value))
			fmt.Printf(`{"jsonrpc":"2.0","id":%s,"result":{"value":%q}}`+"\n", heldResponseID, heldResponseValue)
			heldResponseID = nil
		}
	case "turn/start":
		var p struct {
			Input []InputPart `json:"input"`
		}
		_ = json.Unmarshal(params, &p)
		text := ""
		if len(p.Input) > 0 {
			text = p.Input[0].Text
		}
		if text == "hello" {
			return false
		}
		switch text {
		case "route":
			event("item/started", `{"sessionId":"s1","item":{"itemId":"foreign","turnId":"other"}}`)
			event("item/delta", `{"sessionId":"s1","itemId":"foreign","delta":"wrong"}`)
			event("item/started", `{"sessionId":"s1","item":{"itemId":"i1","turnId":"t1"}}`)
			event("item/delta", `{"sessionId":"s1","itemId":"i1","delta":"hello"}`)
			event("item/completed", `{"sessionId":"s1","item":{"itemId":"i1","turnId":"t1","text":"hello"}}`)
			event("turn/foregroundCompleted", `{"sessionId":"s1","turnId":"other"}`)
			event("turn/completed", `{"sessionId":"s1","turnId":"t1","terminal":"completed"}`)
		case "failed":
			event("turn/completed", `{"sessionId":"s1","turnId":"t1","terminal":"failed","error":{"kind":"providerFailure"}}`)
		case "unqueued":
			event("turn/unqueued", `{"sessionId":"s1","turnId":"t1"}`)
		case "gap":
			event("view/gap", `{"sessionId":"s1","after":"opaque/before","next":"opaque/after"}`)
		}
		reply(`{"turnId":"t1","disposition":"started"}`)
	case "turn/cancel", "turn/interrupt":
		reply(`{}`)
		event("turn/completed", `{"sessionId":"s1","turnId":"t1","terminal":"cancelled"}`)
	case "burst":
		for i := 0; i < 4; i++ {
			event("unknown/future", `{"sessionId":"s1","future":true}`)
		}
		reply(`{}`)
	case "exit-seven":
		os.Exit(7)
	case "no-response":
	case "invalid-json":
		fmt.Println("{")
	case "invalid-version":
		fmt.Printf(`{"jsonrpc":"1.0","id":%s,"result":{}}`+"\n", id)
	case "invalid-response":
		fmt.Printf(`{"jsonrpc":"2.0","id":%s}`+"\n", id)
	case "invalid-id":
		fmt.Println(`{"jsonrpc":"2.0","id":{},"method":"request"}`)
	case "oversized":
		fmt.Println(strings.Repeat("x", maxFrameBytes+1))
	case "view/page":
		var p struct {
			Cursor string `json:"cursor"`
		}
		_ = json.Unmarshal(params, &p)
		switch p.Cursor {
		case "opaque/start":
			reply(`{"events":[{"method":"item/completed","params":{"sessionId":"s1","viewCursor":"opaque/one","item":{"revision":2}}}],"nextCursor":"opaque/one"}`)
		case "loop":
			reply(`{"events":[],"nextCursor":"loop"}`)
		default:
			reply(`{"events":[{"method":"turn/completed","params":{"viewCursor":"opaque/two"}}],"nextCursor":null}`)
		}
	default:
		return false
	}
	return true
}

func TestMultiSessionIsolation(t *testing.T) {
	h := newFake(t)
	a := h.Subscribe("s1", 8)
	defer a.Close()
	b := h.Subscribe("s2", 8)
	defer b.Close()
	if err := h.Call(testContext(t), "two-sessions", nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, s := range []*Subscription{a, b} {
		select {
		case n := <-s.Events():
			id, _, _ := n.Identity()
			if id != s.sessionID {
				t.Fatal(id)
			}
		case <-time.After(time.Second):
			t.Fatal("missing session event")
		}
	}
	if len(a.events) != 0 || len(b.events) != 0 {
		t.Fatal("cross-session delivery")
	}
}
func TestOutOfOrderResponses(t *testing.T) {
	h := newFake(t)
	errCh := make(chan error, 2)
	for _, value := range []string{"first", "second"} {
		go func(value string) {
			var out struct {
				Value string `json:"value"`
			}
			err := h.Call(testContext(t), "reorder", map[string]string{"value": value}, &out)
			if err == nil && out.Value != value {
				err = fmt.Errorf("wanted %s, got %s", value, out.Value)
			}
			errCh <- err
		}(value)
	}
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
	}
}
func TestApprovalAndInputDecoding(t *testing.T) {
	req := Request{Method: "approval/request", Params: json.RawMessage(`{"approvalId":"a1","sessionId":"s1","currentRequirementId":{"approvalId":"a1","sourceIndex":2},"availableChoices":[{"choiceId":"opaque-choice","label":"Deny","decision":"denied","scope":"once"}],"subject":{"kind":"future"},"future":true}`)}
	p, err := req.Approval()
	if err != nil || p.AvailableChoices[0].ChoiceID != "opaque-choice" || !strings.Contains(string(p.Raw), "future") {
		t.Fatalf("%+v %v", p, err)
	}
	req = Request{Method: "userInput/request", Params: json.RawMessage(`{"userInputId":"q1","sessionId":"s1","questions":[{"id":"question-1","question":"Continue?","selection":{"type":"single"},"options":[{"label":"Yes"},{"label":"No"}]}]}`)}
	q, err := req.Input()
	if err != nil || q.Questions[0].ID != "question-1" {
		t.Fatalf("%+v %v", q, err)
	}
	if _, err = req.Approval(); err == nil {
		t.Fatal("wrong request kind accepted")
	}
}

var heldResponseID json.RawMessage
var heldResponseValue string

func TestSubscriptionPayloadOwnership(t *testing.T) {
	h := newFake(t)
	a := h.Subscribe("s1", 8)
	defer a.Close()
	b := h.Subscribe("s1", 8)
	defer b.Close()
	if err := h.Call(testContext(t), "burst", nil, nil); err != nil {
		t.Fatal(err)
	}
	first := <-a.Events()
	second := <-b.Events()
	first.Params[0] = '!'
	first.Raw[0] = '!'
	if !json.Valid(second.Params) || !json.Valid(second.Raw) {
		t.Fatal("subscribers share writable payload storage")
	}
}
func TestAbandonedTurnReleasesTracker(t *testing.T) {
	h := newFake(t)
	turn, err := testSession(t, h).Send(testContext(t), []InputPart{Text("hold")})
	if err != nil {
		t.Fatal(err)
	}
	turn.CloseTracking()
	if _, err = turn.Wait(testContext(t)); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	h.mu.Lock()
	n := len(h.subscribers)
	h.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d subscriptions leaked", n)
	}
	if err = h.Call(testContext(t), "echo", nil, nil); err != nil {
		t.Fatal(err)
	}
}
func TestFailedTerminalRetainsOutcome(t *testing.T) {
	h := newFake(t)
	turn, err := testSession(t, h).Send(testContext(t), []InputPart{Text("failed")})
	if err != nil {
		t.Fatal(err)
	}
	r, err := turn.Wait(testContext(t))
	if err != nil || r.Terminal != "failed" || len(r.Error) == 0 {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestExecCancellationClosesInheritedStdout(t *testing.T) {
	c := NewClient(os.Args[0])
	c.Env = []string{"MUSE_FAKE_HOST=1", "MUSE_FAKE_SCENARIO=inherited-stdout"}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.RunPromptCtx(ctx, "hello", nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("cancellation waited for descendant stdout")
	}
}

func TestHostExitStatusIsPreserved(t *testing.T) {
	h := newFake(t)
	err := h.Call(testContext(t), "exit-seven", nil, nil)
	if err == nil {
		t.Fatal("expected process failure")
	}
	var native *Error
	// The pending-call wrapper is transport; the host retains the native cause.
	if !errors.As(h.Err(), &native) || native.Kind != "process" || native.ExitCode != 7 {
		t.Fatalf("lost native status: %v", h.Err())
	}
}
