package muse

import (
	"bufio"
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

func TestMain(m *testing.M) {
	if os.Getenv("MUSE_FAKE_HOST") == "1" {
		if len(os.Args) > 1 && os.Args[1] == "exec" {
			fakeExec()
			os.Exit(0)
		}
		fakeHost()
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func fakeHost() {
	s := bufio.NewScanner(os.Stdin)
	for s.Scan() {
		var x struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(s.Bytes(), &x) != nil {
			os.Exit(2)
		}
		if len(x.ID) == 0 {
			continue
		}
		switch x.Method {
		case "initialize":
			fmt.Printf(`{"jsonrpc":"2.0","id":%s,"result":{"schema":{"version":1,"fingerprint":"%s"}}}`+"\n", x.ID, knownFingerprint)
		case "session/start":
			fmt.Printf(`{"jsonrpc":"2.0","id":%s,"result":{"session":{"sessionId":"s1"},"viewCursor":"c1"}}`+"\n", x.ID)
		case "turn/start":
			fmt.Println(`{"jsonrpc":"2.0","method":"turn/completed","params":{"sessionId":"s1","turnId":"t1","terminal":"completed"}}`)
			fmt.Printf(`{"jsonrpc":"2.0","id":%s,"result":{"turnId":"t1","disposition":"started"}}`+"\n", x.ID)
		case "failure":
			fmt.Printf(`{"jsonrpc":"2.0","id":%s,"error":{"code":-32053,"message":"stale","data":{"kind":"approvalRequirementStale"}}}`+"\n", x.ID)
		case "pause":
			fmt.Printf(`{"jsonrpc":"2.0","id":%s,"result":{}}`+"\n", x.ID)
			time.Sleep(10 * time.Second)
		case "crash":
			return
		case "approval/decide":
			var p struct {
				SessionID   string `json:"sessionId"`
				ApprovalID  string `json:"approvalId"`
				ChoiceID    string `json:"choiceId"`
				CommandID   string `json:"commandId"`
				Requirement struct {
					SourceIndex int `json:"sourceIndex"`
				} `json:"requirementId"`
			}
			_ = json.Unmarshal(x.Params, &p)
			if p.SessionID != "s1" || p.ApprovalID != "a1" || p.ChoiceID != "deny" || p.Requirement.SourceIndex != 2 || p.CommandID == "" {
				fmt.Printf(`{"jsonrpc":"2.0","id":%s,"error":{"code":-32602,"message":"bad approval"}}`+"\n", x.ID)
			} else {
				fmt.Printf(`{"jsonrpc":"2.0","id":%s,"result":{}}`+"\n", x.ID)
			}
		case "userInput/answer":
			var p struct {
				UserInputID string `json:"userInputId"`
				Answers     []struct {
					QuestionID string `json:"questionId"`
					FreeText   string `json:"freeText"`
				} `json:"answers"`
			}
			_ = json.Unmarshal(x.Params, &p)
			if p.UserInputID != "q1" || len(p.Answers) != 1 || p.Answers[0].QuestionID != "question-1" || p.Answers[0].FreeText != "yes" {
				fmt.Printf(`{"jsonrpc":"2.0","id":%s,"error":{"code":-32602,"message":"bad answer"}}`+"\n", x.ID)
			} else {
				fmt.Printf(`{"jsonrpc":"2.0","id":%s,"result":{}}`+"\n", x.ID)
			}
		case "question":
			fmt.Println(`{"jsonrpc":"2.0","id":"request-1","method":"approval/request","params":{"approvalId":"a1"}}`)
			fmt.Printf(`{"jsonrpc":"2.0","id":%s,"result":{}}`+"\n", x.ID)
		default:
			fmt.Printf(`{"jsonrpc":"2.0","id":%s,"result":{"ok":true}}`+"\n", x.ID)
		}
	}
}
func newFake(t *testing.T) *MSPHost {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h, err := NewClient(os.Args[0]).StartMSP(ctx, HostOptions{Env: []string{"MUSE_FAKE_HOST=1"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := h.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return h
}
func TestTurnCompletionBeforeAcknowledgement(t *testing.T) {
	h := newFake(t)
	s, err := h.StartSession(context.Background(), SessionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := s.Send(context.Background(), []InputPart{Text("hello")})
	if err != nil {
		t.Fatal(err)
	}
	r, err := turn.Wait(context.Background())
	if err != nil || r.Terminal != "completed" {
		t.Fatalf("result %#v: %v", r, err)
	}
}
func TestConcurrentCallsAndRemoteError(t *testing.T) {
	h := newFake(t)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out struct {
				OK bool `json:"ok"`
			}
			if err := h.Call(context.Background(), "echo", nil, &out); err != nil || !out.OK {
				t.Errorf("call: %+v %v", out, err)
			}
		}()
	}
	wg.Wait()
	err := h.Call(context.Background(), "failure", nil, nil)
	e, ok := err.(*Error)
	if !ok || e.Code != -32053 {
		t.Fatalf("unexpected error %v", err)
	}
}
func TestApprovalRequestRequiresExplicitReceipt(t *testing.T) {
	h := newFake(t)
	if err := h.Call(context.Background(), "question", nil, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case req := <-h.Requests():
		if req.Method != "approval/request" {
			t.Fatal(req.Method)
		}
		if err := req.Respond(nil); err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("request missing")
	}
}
func TestHostExitFailsPendingCall(t *testing.T) {
	h := newFake(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.Call(ctx, "crash", nil, nil); err == nil {
		t.Fatal("expected transport error")
	}
}

func fakeExec() {
	switch os.Getenv("MUSE_FAKE_SCENARIO") {
	case "invalid":
		fmt.Println("not json")
		return
	case "missing":
		fmt.Println(`{"payload_type":"task.lifecycle.failed","payload":{"kind":"failed"}}`)
		return
	}
	fmt.Println(`{"payload_type":"task.lifecycle.failed","payload":{"kind":"failed"}}`)
	fmt.Println(`{"stream":{"id":"session-1"},"payload_type":"run.terminal.completed","payload":{"terminal":"completed","text":"hello"}}`)
}
func TestExecResultIgnoresUnrelatedTaskFailure(t *testing.T) {
	c := NewClient(os.Args[0])
	c.Env = []string{"MUSE_FAKE_HOST=1"}
	r, err := c.RunPromptCtx(context.Background(), "hello", nil)
	if err != nil || r.Text != "hello" || r.SessionID != "session-1" {
		t.Fatalf("result=%+v err=%v", r, err)
	}
}
func TestExecRejectsBadStream(t *testing.T) {
	for _, scenario := range []string{"invalid", "missing"} {
		t.Run(scenario, func(t *testing.T) {
			c := NewClient(os.Args[0])
			c.Env = []string{"MUSE_FAKE_HOST=1", "MUSE_FAKE_SCENARIO=" + scenario}
			if _, err := c.RunPromptCtx(context.Background(), "hello", nil); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestCancelledBlockedWriteStopsHost(t *testing.T) {
	h := newFake(t)
	if err := h.Call(context.Background(), "pause", nil, nil); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := h.Call(ctx, "echo", map[string]string{"data": strings.Repeat("x", 4<<20)}, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, got %v", err)
	}
	select {
	case <-h.Done():
	case <-time.After(time.Second):
		t.Fatal("host did not terminate")
	}
}

func TestDecisionsCarryServerIdentifiers(t *testing.T) {
	h := newFake(t)
	s, err := h.StartSession(context.Background(), SessionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DecideApproval(context.Background(), "a1", "deny", json.RawMessage(`{"sourceIndex":2}`)); err != nil {
		t.Fatal(err)
	}
	answers := []map[string]string{{"questionId": "question-1", "freeText": "yes"}}
	if err := s.AnswerInput(context.Background(), "q1", answers); err != nil {
		t.Fatal(err)
	}
	if err := s.DecideApproval(context.Background(), "a1", "", json.RawMessage(`{}`)); err == nil {
		t.Fatal("missing choice should fail")
	}
}
