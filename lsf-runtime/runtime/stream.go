package lsfrt

import (
	"encoding/json"
	"fmt"
)

// streamEvent is the union of the stream-json fields the runtime reads.
// Decoding is lenient: the stream carries many more fields, varying by version.
type streamEvent struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	SessionID string `json:"session_id"`

	// system/init
	SlashCommands []string `json:"slash_commands"`

	// assistant: error is one of the CLI's assistant-message error codes.
	Error   string          `json:"error"`
	Message json.RawMessage `json:"message"`

	// result
	IsError           bool            `json:"is_error"`
	APIErrorStatus    *int            `json:"api_error_status"`
	StructuredOutput  json.RawMessage `json:"structured_output"`
	TotalCostUSD      float64         `json:"total_cost_usd"`
	TerminalReason    string          `json:"terminal_reason"`
	PermissionDenials []struct {
		ToolName  string          `json:"tool_name"`
		ToolInput json.RawMessage `json:"tool_input"`
	} `json:"permission_denials"`

	// system api_retry
	ErrorStatus int `json:"error_status"`

	// rate_limit_event
	RateLimitInfo *struct {
		Status        string `json:"status"`
		ResetsAt      int64  `json:"resetsAt"`
		RateLimitType string `json:"rateLimitType"`
	} `json:"rate_limit_info"`
}

// resultEvent is the final result event's fields.
type resultEvent struct {
	IsError          bool
	APIErrorStatus   int // 0 when absent
	StructuredOutput json.RawMessage
	CostUSD          float64
	TerminalReason   string
	Subtype          string
	Denials          []string
}

// streamAcc accumulates what one attempt's stream showed.
type streamAcc struct {
	SessionID       string
	SlashCommands   []string
	Result          *resultEvent
	AssistantErrors []string
	SkillTools      []string // input.skill of each Skill tool_use
	// RejectedResetsAt is the resetsAt of the last rate_limit_event with
	// status rejected, in unix seconds; 0 when none.
	RejectedResetsAt int64
	// AuthRetries counts api_retry events for a 401: the CLI's own retries
	// of a failed authentication, before any result.
	AuthRetries int
	BadLines    int
}

func (a *streamAcc) add(line []byte) {
	if len(line) == 0 {
		return
	}
	e, ok := decodeEvent(line)
	if !ok {
		a.BadLines++
		return
	}
	switch e.Type {
	case "system":
		switch {
		case e.Subtype == "init":
			a.SessionID = e.SessionID
			a.SlashCommands = e.SlashCommands
		case e.Subtype == "api_retry" && e.ErrorStatus == 401:
			a.AuthRetries++
		}
	case "assistant":
		if e.Error != "" {
			a.AssistantErrors = append(a.AssistantErrors, e.Error)
		}
		a.SkillTools = append(a.SkillTools, skillToolUses(e.Message)...)
	case "rate_limit_event":
		if e.RateLimitInfo != nil && e.RateLimitInfo.Status == "rejected" && e.RateLimitInfo.ResetsAt > 0 {
			a.RejectedResetsAt = e.RateLimitInfo.ResetsAt
		}
	case "result":
		r := &resultEvent{
			IsError:          e.IsError,
			StructuredOutput: e.StructuredOutput,
			CostUSD:          e.TotalCostUSD,
			TerminalReason:   e.TerminalReason,
			Subtype:          e.Subtype,
		}
		if e.APIErrorStatus != nil {
			r.APIErrorStatus = *e.APIErrorStatus
		}
		for _, d := range e.PermissionDenials {
			r.Denials = append(r.Denials, fmt.Sprintf("%s: %s", d.ToolName, truncateRunes(string(d.ToolInput), 160)))
		}
		a.Result = r
	}
}

// skillToolUses returns input.skill of each Skill tool_use in an assistant
// message. A message whose content is not a block list yields none.
func skillToolUses(msg json.RawMessage) []string {
	var m struct {
		Content []struct {
			Type  string `json:"type"`
			Name  string `json:"name"`
			Input struct {
				Skill string `json:"skill"`
			} `json:"input"`
		} `json:"content"`
	}
	if json.Unmarshal(msg, &m) != nil {
		return nil
	}
	var out []string
	for _, b := range m.Content {
		if b.Type == "tool_use" && b.Name == "Skill" && b.Input.Skill != "" {
			out = append(out, b.Input.Skill)
		}
	}
	return out
}

// skillLoaded reports whether the stream shows skill loaded. A slash-command
// expansion is not echoed on stdout (step-0 probes, 2.1.289); init's
// slash_commands lists every command that expands, and an unknown one is sent
// as plain text. So a prefixed prompt loaded its skill iff the command is
// listed; a Skill tool load counts either way.
func (a *streamAcc) skillLoaded(skill string, prefixed bool) bool {
	for _, s := range a.SkillTools {
		if s == skill {
			return true
		}
	}
	if prefixed {
		for _, s := range a.SlashCommands {
			if s == skill {
				return true
			}
		}
	}
	return false
}

// decodeEvent decodes a stream line. A field whose type has drifted (a CLI
// version changing a field's shape) drops only that field, not the line, so a
// result event with one odd field still yields its structured_output. ok is
// false only for a line that is not a JSON object.
func decodeEvent(line []byte) (e streamEvent, ok bool) {
	if json.Unmarshal(line, &e) == nil {
		return e, true
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(line, &fields) != nil {
		return streamEvent{}, false
	}
	e = streamEvent{}
	for k, v := range fields {
		one, _ := json.Marshal(map[string]json.RawMessage{k: v})
		var f streamEvent
		if json.Unmarshal(one, &f) == nil {
			json.Unmarshal(one, &e)
		}
	}
	return e, true
}
