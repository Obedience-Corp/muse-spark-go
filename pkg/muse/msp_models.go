package muse

import (
	"context"
	"encoding/json"
	"fmt"
)

// Model catalog sources reported by model/list. The set is open: an unknown
// value is an unknown source, not an error.
const (
	ModelSourceProvider   = "providerCatalog"
	ModelSourceFake       = "fakeCatalog"
	ModelSourceUnresolved = "unresolvedCatalog"
	ModelSourceBundled    = "bundledCatalog"
	ModelSourceConfig     = "configCatalog"
)

// ModelList is one model/list snapshot, newest release first. Models may be
// empty. Raw preserves future fields. Warnings repeats the host's startup
// warnings, such as an untested schema fingerprint.
type ModelList struct {
	Models     []ModelCatalogEntry `json:"models"`
	ProviderID string              `json:"providerId"`
	ProfileID  *string             `json:"profileId"`
	Source     string              `json:"source"`
	Raw        json.RawMessage     `json:"-"`
	Warnings   []string            `json:"-"`
}

// ModelCatalogEntry is one selectable catalog row. Pointer fields are nil when
// the catalog declared nothing. IsDefault and IsActive may be false on every row.
type ModelCatalogEntry struct {
	ModelID                string     `json:"modelId"`
	DisplayLabel           string     `json:"displayLabel"`
	ProviderID             string     `json:"providerId"`
	ProfileID              *string    `json:"profileId"`
	IsDefault              bool       `json:"isDefault"`
	IsActive               bool       `json:"isActive"`
	Description            *string    `json:"description"`
	ReleaseDate            *string    `json:"releaseDate"`
	ContextLimit           *int64     `json:"contextLimit"`
	OutputLimit            *int64     `json:"outputLimit"`
	Cost                   *ModelCost `json:"cost"`
	DefaultReasoningEffort string     `json:"defaultReasoningEffort,omitempty"`
	// ReasoningEffortVariants describes only the tiers the catalog labels: a
	// same-order subset of Variants that may be empty. Use Variants for choices.
	ReasoningEffortVariants []ModelReasoningEffort `json:"reasoningEffortVariants,omitempty"`
	// Variants is the complete ordered set of selectable efforts. Nil when an
	// older host omits it.
	Variants *ModelEffortVariants `json:"variants,omitempty"`
}

// ModelEffortVariants is a row's complete ordered effort set. Unknown is true
// when the host declares the capability unknown; Tiers is then empty.
type ModelEffortVariants struct {
	Tiers   []string
	Unknown bool
}

func (v *ModelEffortVariants) UnmarshalJSON(b []byte) error {
	if string(b) == `"unknown"` {
		*v = ModelEffortVariants{Unknown: true}
		return nil
	}
	var tiers []string
	if err := json.Unmarshal(b, &tiers); err != nil {
		return fmt.Errorf("effort variants are neither a tier array nor \"unknown\": %w", err)
	}
	*v = ModelEffortVariants{Tiers: tiers}
	return nil
}

func (v ModelEffortVariants) MarshalJSON() ([]byte, error) {
	if v.Unknown {
		return []byte(`"unknown"`), nil
	}
	if v.Tiers == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(v.Tiers)
}

// ModelCost is the per-1M-token catalog cost, carried verbatim as decimal strings.
type ModelCost struct {
	Input    string  `json:"input"`
	Output   string  `json:"output"`
	Cached   string  `json:"cached"`
	Currency *string `json:"currency"`
}

// ModelReasoningEffort is one described reasoning-effort tier of a catalog row.
type ModelReasoningEffort struct {
	Tier        string `json:"tier"`
	Description string `json:"description,omitempty"`
}

// ListModels reads the host's model catalog. A non-empty sessionID flags the
// row matching that session's effective model as active. It is a query: it
// records no command and emits no view event.
func (h *MSPHost) ListModels(ctx context.Context, sessionID string) (ModelList, error) {
	params := map[string]any{}
	if sessionID != "" {
		params["sessionId"] = sessionID
	}
	var raw json.RawMessage
	if err := h.Call(ctx, "model/list", params, &raw); err != nil {
		return ModelList{}, err
	}
	list, err := decodeModelList(raw)
	if err != nil {
		return ModelList{}, err
	}
	list.Warnings = append([]string(nil), h.Warnings...)
	return list, nil
}

// ListModels starts an owned MSP host with o, reads its model catalog and
// closes the host. The catalog depends on the provider, so set o.Provider to
// the provider the caller will run. The child is reaped before return.
func (c *Client) ListModels(ctx context.Context, o HostOptions) (ModelList, error) {
	list, _, err := c.listModels(ctx, o)
	return list, err
}

func (c *Client) listModels(ctx context.Context, o HostOptions) (ModelList, *MSPHost, error) {
	h, err := c.StartMSP(ctx, o)
	if err != nil {
		return ModelList{}, nil, err
	}
	list, err := h.ListModels(ctx, "")
	if err != nil {
		h.abortStartup(err)
		return ModelList{}, h, err
	}
	// The catalog is already read. Close within ctx: an expired ctx kills and
	// reaps the child, which is not the caller's failure.
	_ = h.Close(ctx)
	return list, h, nil
}

func decodeModelList(raw json.RawMessage) (ModelList, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || len(fields["models"]) == 0 || string(fields["models"]) == "null" {
		return ModelList{}, &Error{Kind: "protocol", Message: "incomplete model list", Err: err}
	}
	var wire struct {
		Models     []ModelCatalogEntry `json:"models"`
		ProviderID string              `json:"providerId"`
		ProfileID  *string             `json:"profileId"`
		Source     string              `json:"source"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return ModelList{}, &Error{Kind: "protocol", Message: "decode model list", Err: err}
	}
	for _, m := range wire.Models {
		if m.ModelID == "" {
			return ModelList{}, &Error{Kind: "protocol", Message: "model list row has no model id"}
		}
	}
	if wire.Models == nil {
		wire.Models = []ModelCatalogEntry{}
	}
	return ModelList{Models: wire.Models, ProviderID: wire.ProviderID, ProfileID: wire.ProfileID, Source: wire.Source, Raw: raw}, nil
}
