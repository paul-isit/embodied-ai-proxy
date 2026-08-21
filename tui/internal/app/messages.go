package app

import (
	"embodied-ai-proxy/tui/internal/client"
	"encoding/json"
	"time"
)

// SystemInfoMsg delivers the result of the initial GET /api/info call
type SystemInfoMsg struct {
	Info *client.SystemInfo
	Err  error
}

type WSConnectedMsg struct{}

type WSDisconnectedMsg struct {
	Err error
}

// StatusUpdateMsg represents a "status_update" envelop from backend/bridge
type StatusUpdateMsg struct {
	BridgeConnected *bool           `json:"bridge_connected,omitempty"`
	ObjectList      []string        `json:"object_list,omitempty"`
	ExecutionResult json.RawMessage `json:"execution_result,omitempty"`
	Raw             json.RawMessage
}

// ActionStep is a single step of a validated action recipe, matching
// data/config/json_schema.json's "steps" item shape.
type ActionStep struct {
	StepID      int            `json:"step_id"`
	Action      string         `json:"action"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// ActionRecipeMsg represents a validated "action_recipe" envelope, which per
// data/config/json_schema.json is either a success document (status,
// recipe_name, steps) or an error document (status, error_type, message).
type ActionRecipeMsg struct {
	Status     string       `json:"status"`
	RecipeName string       `json:"recipe_name,omitempty"`
	Steps      []ActionStep `json:"steps,omitempty"`
	ErrorType  string       `json:"error_type,omitempty"`
	Message    string       `json:"message,omitempty"`
	RawOutput  string       `json:"raw_output,omitempty"`
}

// LogEventMsg represents a "log_event" envelope
type LogEventMsg struct {
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// PromptProcessingMsg indicates whether a user command is currently being processed
type PromptProcessingMsg struct {
	InProcess bool
	Prompt    string
}
