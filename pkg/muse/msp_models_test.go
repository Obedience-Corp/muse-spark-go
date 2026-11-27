package muse

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

const fakeModelRows = `[
 {"modelId":"alpha-2","displayLabel":"Alpha 2","providerId":"acme","profileId":null,"isDefault":false,"isActive":%t,
  "description":"Second alpha","releaseDate":"2026-02-01","contextLimit":200000,"outputLimit":32000,
  "cost":{"input":"1.25","output":"10","cached":"0.125","currency":"USD"},
  "defaultReasoningEffort":"high","reasoningEffortVariants":[{"tier":"low","description":"Fast"},{"tier":"high"}],
  "variants":["low","high"],"futureField":1},
 {"modelId":"alpha-1","displayLabel":"Alpha 1","providerId":"acme","profileId":"team","isDefault":true,"isActive":false,
  "description":null,"releaseDate":null,"contextLimit":null,"outputLimit":null,"cost":null,"variants":"unknown"}
]`

func fakeModelList(id, params json.RawMessage) {
	reply := func(result string) { fmt.Printf(`{"jsonrpc":"2.0","id":%s,"result":%s}`+"\n", id, result) }
	var p struct {
		SessionID *string `json:"sessionId"`
	}
	_ = json.Unmarshal(params, &p)
	switch os.Getenv("MUSE_FAKE_MODELS") {
	case "empty":
		reply(`{"models":[],"profileId":null,"providerId":"acme","source":"bundledCatalog"}`)
	case "null":
		reply(`{"models":null,"profileId":null,"providerId":"acme","source":"providerCatalog"}`)
	case "bad-variants":
		reply(`{"models":[{"modelId":"m","displayLabel":"m","providerId":"acme","profileId":null,"isDefault":false,"isActive":false,"description":null,"releaseDate":null,"contextLimit":null,"outputLimit":null,"cost":null,"variants":"UNKNOWN"}],"profileId":null,"providerId":"acme","source":"providerCatalog"}`)
	case "no-variants":
		reply(`{"models":[{"modelId":"m","displayLabel":"m","providerId":"acme","profileId":null,"isDefault":false,"isActive":false,"description":null,"releaseDate":null,"contextLimit":null,"outputLimit":null,"cost":null}],"profileId":null,"providerId":"acme","source":"providerCatalog"}`)
	case "no-id":
		reply(`{"models":[{"modelId":"","displayLabel":"x"}],"profileId":null,"providerId":"acme","source":"providerCatalog"}`)
	case "unsupported":
		fmt.Printf(`{"jsonrpc":"2.0","id":%s,"error":{"code":-32601,"message":"method not found"}}`+"\n", id)
	case "hang":
		time.Sleep(10 * time.Second)
	default:
		if p.SessionID != nil && *p.SessionID == "" {
			reply(`{"models":"sent an empty session id"}`)
			return
		}
		active := p.SessionID != nil && *p.SessionID == "s1"
		var frame bytes.Buffer
		_ = json.Compact(&frame, []byte(fmt.Sprintf(`{"models":`+fakeModelRows+`,"profileId":null,"providerId":"acme","source":"someFutureCatalog"}`, active)))
		reply(frame.String())
	}
}

func fakeModels(scenario string) HostOptions {
	return HostOptions{Env: []string{"MUSE_FAKE_HOST=1", "MUSE_FAKE_MODELS=" + scenario}}
}

func TestListModelsDecodesCatalog(t *testing.T) {
	list, err := NewClient(os.Args[0]).ListModels(testContext(t), fakeModels("full"))
	if err != nil {
		t.Fatal(err)
	}
	if list.ProviderID != "acme" || list.ProfileID != nil || list.Source != "someFutureCatalog" || len(list.Raw) == 0 {
		t.Fatalf("catalog metadata: %+v", list)
	}
	if len(list.Models) != 2 {
		t.Fatalf("rows: %+v", list.Models)
	}
	a, b := list.Models[0], list.Models[1]
	if a.ModelID != "alpha-2" || a.DisplayLabel != "Alpha 2" || a.IsDefault || a.IsActive || *a.Description != "Second alpha" || *a.ReleaseDate != "2026-02-01" || *a.ContextLimit != 200000 || *a.OutputLimit != 32000 {
		t.Fatalf("first row: %+v", a)
	}
	if a.Cost == nil || a.Cost.Input != "1.25" || a.Cost.Output != "10" || a.Cost.Cached != "0.125" || *a.Cost.Currency != "USD" {
		t.Fatalf("cost: %+v", a.Cost)
	}
	if a.DefaultReasoningEffort != "high" || len(a.ReasoningEffortVariants) != 2 || a.ReasoningEffortVariants[0] != (ModelReasoningEffort{Tier: "low", Description: "Fast"}) || a.Variants == nil || a.Variants.Unknown || !slices.Equal(a.Variants.Tiers, []string{"low", "high"}) {
		t.Fatalf("efforts: %+v", a)
	}
	if b.ModelID != "alpha-1" || !b.IsDefault || *b.ProfileID != "team" || b.Description != nil || b.ReleaseDate != nil || b.ContextLimit != nil || b.OutputLimit != nil || b.Cost != nil || b.DefaultReasoningEffort != "" || b.ReasoningEffortVariants != nil || b.Variants == nil || !b.Variants.Unknown || b.Variants.Tiers != nil {
		t.Fatalf("null row: %+v", b)
	}
}

func TestListModelsSessionMarksActive(t *testing.T) {
	h, err := NewClient(os.Args[0]).StartMSP(testContext(t), fakeModels("full"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close(context.Background()) })
	without, err := h.ListModels(testContext(t), "")
	if err != nil {
		t.Fatal(err)
	}
	with, err := h.ListModels(testContext(t), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if without.Models[0].IsActive || !with.Models[0].IsActive {
		t.Fatalf("active row: without=%v with=%v", without.Models[0].IsActive, with.Models[0].IsActive)
	}
}

func TestListModelsEmptyCatalog(t *testing.T) {
	list, err := NewClient(os.Args[0]).ListModels(testContext(t), fakeModels("empty"))
	if err != nil {
		t.Fatal(err)
	}
	if list.Models == nil || len(list.Models) != 0 || list.Source != ModelSourceBundled {
		t.Fatalf("empty catalog: %+v", list)
	}
}

func TestListModelsRejectsMalformedCatalog(t *testing.T) {
	for _, scenario := range []string{"null", "no-id", "bad-variants"} {
		t.Run(scenario, func(t *testing.T) {
			_, err := NewClient(os.Args[0]).ListModels(testContext(t), fakeModels(scenario))
			var e *Error
			if !errors.As(err, &e) || e.Kind != "protocol" {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestListModelsUnsupportedHost(t *testing.T) {
	list, h, err := NewClient(os.Args[0]).listModels(testContext(t), fakeModels("unsupported"))
	var e *Error
	if !errors.As(err, &e) || e.Kind != "remote" || e.Code != -32601 || list.Models != nil {
		t.Fatalf("got %v %+v", err, list)
	}
	assertReaped(t, h)
}

func TestListModelsCancellationReapsHost(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, h, err := NewClient(os.Args[0]).listModels(ctx, fakeModels("hang"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("cancelled listing waited for the host")
	}
	assertReaped(t, h)
}

func TestListModelsClosesHostOnSuccess(t *testing.T) {
	_, h, err := NewClient(os.Args[0]).listModels(testContext(t), fakeModels("full"))
	if err != nil {
		t.Fatal(err)
	}
	assertReaped(t, h)
}

func assertReaped(t *testing.T, h *MSPHost) {
	t.Helper()
	if h == nil {
		t.Fatal("no host started")
	}
	select {
	case <-h.processDone:
	default:
		t.Fatal("child not reaped")
	}
}

func TestListModelsOlderHostOmitsVariants(t *testing.T) {
	list, err := NewClient(os.Args[0]).ListModels(testContext(t), fakeModels("no-variants"))
	if err != nil {
		t.Fatal(err)
	}
	if list.Models[0].Variants != nil {
		t.Fatalf("variants = %+v, want nil", list.Models[0].Variants)
	}
}

func TestListModelsCarriesHostWarnings(t *testing.T) {
	o := fakeModels("full")
	o.Env = append(o.Env, "MUSE_FAKE_SCENARIO=fingerprint")
	list, err := NewClient(os.Args[0]).ListModels(testContext(t), o)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Warnings) != 1 || !strings.Contains(list.Warnings[0], "fingerprint") {
		t.Fatalf("warnings = %q", list.Warnings)
	}
}

func TestModelListEncodesWireNames(t *testing.T) {
	list, err := NewClient(os.Args[0]).ListModels(testContext(t), fakeModels("full"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	var again ModelList
	if err := json.Unmarshal(b, &again); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"models":`, `"providerId":"acme"`, `"source":"someFutureCatalog"`, `"variants":["low","high"]`, `"variants":"unknown"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("encoded list lacks %s: %s", want, b)
		}
	}
	if !again.Models[1].Variants.Unknown || !slices.Equal(again.Models[0].Variants.Tiers, []string{"low", "high"}) {
		t.Fatalf("round trip = %+v", again.Models)
	}
}
