package muse

import "encoding/json"

// ApprovalRequest is the presentation payload, not a decision. Preserve Raw and
// CurrentRequirementID when forwarding unknown subjects or future fields.
type ApprovalRequest struct {
	ApprovalID           string           `json:"approvalId"`
	SessionID            string           `json:"sessionId"`
	TurnID               string           `json:"turnId"`
	ToolName             string           `json:"toolName"`
	RawArgs              string           `json:"rawArgs"`
	CurrentRequirementID json.RawMessage  `json:"currentRequirementId"`
	AvailableChoices     []ApprovalChoice `json:"availableChoices"`
	Subject              json.RawMessage  `json:"subject"`
	Raw                  json.RawMessage  `json:"-"`
}
type ApprovalChoice struct {
	ChoiceID        string `json:"choiceId"`
	Label           string `json:"label"`
	Decision        string `json:"decision"`
	Scope           string `json:"scope"`
	RulePreview     string `json:"rulePreview,omitempty"`
	AcceptsFeedback bool   `json:"acceptsFeedback,omitempty"`
}
type InputRequest struct {
	UserInputID string          `json:"userInputId"`
	SessionID   string          `json:"sessionId"`
	TurnID      string          `json:"turnId"`
	Questions   []InputQuestion `json:"questions"`
	Raw         json.RawMessage `json:"-"`
}
type InputQuestion struct {
	ID        string          `json:"id"`
	Header    string          `json:"header"`
	Question  string          `json:"question"`
	Selection json.RawMessage `json:"selection"`
	Options   []InputOption   `json:"options"`
}
type InputOption struct {
	Label       string          `json:"label"`
	Description string          `json:"description,omitempty"`
	Preview     json.RawMessage `json:"preview,omitempty"`
}
type InputAnswer struct {
	QuestionID     string   `json:"questionId"`
	FreeText       string   `json:"freeText,omitempty"`
	SelectedLabel  string   `json:"selectedLabel,omitempty"`
	SelectedLabels []string `json:"selectedLabels,omitempty"`
	Note           string   `json:"note,omitempty"`
}

func (r Request) Approval() (ApprovalRequest, error) {
	var p ApprovalRequest
	if r.Method != "approval/request" {
		return p, &Error{Kind: "validation", Message: "not an approval request"}
	}
	if err := json.Unmarshal(r.Params, &p); err != nil {
		return p, &Error{Kind: "protocol", Message: "decode approval", Err: err}
	}
	if p.ApprovalID == "" || p.SessionID == "" || len(p.AvailableChoices) == 0 || len(p.CurrentRequirementID) == 0 {
		return p, &Error{Kind: "protocol", Message: "incomplete approval request"}
	}
	p.Raw = append(json.RawMessage(nil), r.Params...)
	return p, nil
}
func (r Request) Input() (InputRequest, error) {
	var p InputRequest
	if r.Method != "userInput/request" {
		return p, &Error{Kind: "validation", Message: "not a user input request"}
	}
	if err := json.Unmarshal(r.Params, &p); err != nil {
		return p, &Error{Kind: "protocol", Message: "decode input request", Err: err}
	}
	if p.UserInputID == "" || p.SessionID == "" || len(p.Questions) == 0 {
		return p, &Error{Kind: "protocol", Message: "incomplete user input request"}
	}
	p.Raw = append(json.RawMessage(nil), r.Params...)
	return p, nil
}
