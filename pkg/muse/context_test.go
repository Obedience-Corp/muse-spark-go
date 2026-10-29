package muse

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"
)

type readyWriter struct {
	once  sync.Once
	ready chan struct{}
}

func (w *readyWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.ready) })
	return len(p), nil
}
func awaitReady(t *testing.T, ready <-chan struct{}) {
	t.Helper()
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("fake process did not reach blocking operation")
	}
}
func requireCause(t *testing.T, err, category, cause error) {
	t.Helper()
	if !errors.Is(err, category) || !errors.Is(err, cause) {
		t.Fatalf("wanted category %v and cause %v; got %v", category, cause, err)
	}
}

func TestVersionPreservesCancellationCause(t *testing.T) {
	c := NewClient(os.Args[0])
	c.Env = []string{"MUSE_FAKE_HOST=1"}
	ready := &readyWriter{ready: make(chan struct{})}
	c.Stderr = ready
	cause := errors.New("version probe abandoned")
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	result := make(chan error, 1)
	go func() { _, err := c.Version(ctx); result <- err }()
	awaitReady(t, ready.ready)
	cancel(cause)
	select {
	case err := <-result:
		requireCause(t, err, context.Canceled, cause)
	case <-time.After(time.Second):
		t.Fatal("version ignored cancellation")
	}
}
func TestStartupCancellationReapsWithoutFreshGracePeriod(t *testing.T) {
	c := NewClient(os.Args[0])
	ready := &readyWriter{ready: make(chan struct{})}
	cause := errors.New("startup abandoned")
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	result := make(chan error, 1)
	go func() {
		h, err := c.StartMSP(ctx, HostOptions{Env: []string{"MUSE_FAKE_HOST=1", "MUSE_FAKE_SCENARIO=startup-hang"}, Stderr: ready})
		if h != nil {
			_ = h.Close(context.Background())
		}
		result <- err
	}()
	awaitReady(t, ready.ready)
	cancel(cause)
	select {
	case err := <-result:
		requireCause(t, err, context.Canceled, cause)
	case <-time.After(time.Second):
		t.Fatal("startup spent a fresh cleanup grace period after cancellation")
	}
}
func TestStartupContextDoesNotOwnReturnedHost(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h, err := NewClient(os.Args[0]).StartMSP(ctx, HostOptions{Env: []string{"MUSE_FAKE_HOST=1"}})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close(context.Background())
	cancel()
	if err = h.Call(testContext(t), "echo", nil, nil); err != nil {
		t.Fatalf("startup context killed returned host: %v", err)
	}
}
func TestReceiptGateHonorsCallerDeadline(t *testing.T) {
	h := newFake(t)
	if err := h.Call(testContext(t), "question", nil, nil); err != nil {
		t.Fatal(err)
	}
	r := <-h.Requests()
	// Simulate another response owning this request's receipt gate. The waiting
	// caller must leave without sending anything or waiting for that owner.
	r.state.gate <- struct{}{}
	cause := errors.New("presentation deadline")
	ctx, cancel := context.WithTimeoutCause(context.Background(), 50*time.Millisecond, cause)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- r.RespondContext(ctx, nil) }()
	select {
	case err := <-result:
		requireCause(t, err, context.DeadlineExceeded, cause)
	case <-time.After(time.Second):
		<-r.state.gate
		t.Fatal("receipt gate ignored deadline")
	}
	<-r.state.gate
	if err := r.RespondContext(testContext(t), nil); err != nil {
		t.Fatalf("cancelled waiter consumed the receipt: %v", err)
	}
}
func TestRPCWaitCancellationDetachesOnlyItsWaiter(t *testing.T) {
	h := newFake(t)
	cause := errors.New("RPC caller abandoned")
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	result := make(chan error, 1)
	sub := h.Subscribe("", 1)
	defer sub.Close()
	go func() { result <- h.Call(ctx, "no-response-observed", nil, nil) }()
	select {
	case <-sub.Events():
	case <-time.After(time.Second):
		t.Fatal("RPC did not reach peer")
	}
	// The peer continues servicing unrelated requests while this call waits.
	if err := h.Call(testContext(t), "echo", nil, nil); err != nil {
		t.Fatal(err)
	}
	cancel(cause)
	requireCause(t, <-result, context.Canceled, cause)
	h.mu.Lock()
	pending := len(h.pending)
	h.mu.Unlock()
	if pending != 0 {
		t.Fatalf("%d pending waiters leaked", pending)
	}
	if err := h.Call(testContext(t), "echo", nil, nil); err != nil {
		t.Fatal(err)
	}
}
func TestWriterAcquisitionPreservesDeadlineCause(t *testing.T) {
	h := newFake(t)
	h.writer <- struct{}{}
	cause := errors.New("notification deadline")
	ctx, cancel := context.WithTimeoutCause(context.Background(), 50*time.Millisecond, cause)
	defer cancel()
	err := h.NotifyContext(ctx, "note", nil)
	<-h.writer
	requireCause(t, err, context.DeadlineExceeded, cause)
	if err = h.Call(testContext(t), "echo", nil, nil); err != nil {
		t.Fatal(err)
	}
}
func TestReplayCancellationFromFinalVisitor(t *testing.T) {
	h := newFake(t)
	s := testSession(t, h)
	cause := errors.New("view consumer stopped")
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	err := s.ReplayView(ctx, "opaque/one", func(Notification) error { cancel(cause); return nil })
	requireCause(t, err, context.Canceled, cause)
}
func TestStreamPreservesCancellationCause(t *testing.T) {
	c := NewClient(os.Args[0])
	c.Env = []string{"MUSE_FAKE_HOST=1", "MUSE_FAKE_SCENARIO=flood"}
	cause := errors.New("stream consumer left")
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	events, errs := c.StreamPrompt(ctx, "hello", nil)
	<-events
	cancel(cause)
	for range events {
	}
	requireCause(t, <-errs, context.Canceled, cause)
}
func TestTurnWaitPreservesCauseWithoutCancellingNativeTurn(t *testing.T) {
	h := newFake(t)
	turn, err := testSession(t, h).Send(testContext(t), []InputPart{Text("hold")})
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("view left")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	_, err = turn.Wait(ctx)
	requireCause(t, err, context.Canceled, cause)
	if err = turn.Cancel(testContext(t)); err != nil {
		t.Fatal(err)
	}
	r, err := turn.Wait(testContext(t))
	if err != nil || r.Terminal != "cancelled" {
		t.Fatalf("%+v %v", r, err)
	}
}
func TestClosePreservesCancellationCauseAndReaps(t *testing.T) {
	h := newFake(t)
	if err := h.Call(testContext(t), "pause", nil, nil); err != nil {
		t.Fatal(err)
	}
	cause := errors.New("service shutdown")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	requireCause(t, h.Close(ctx), context.Canceled, cause)
	select {
	case <-h.processDone:
	default:
		t.Fatal("Close returned without reaping")
	}
}
