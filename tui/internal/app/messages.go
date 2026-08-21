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

// ActionRecipeMsg represents a validated "action_recipe" envelope
type ActionRecipeMsg struct {
	Status    string            `json:"status"`
	Summary   string            `json:"summary,omitempty"`
	Steps     []ActionRecipeMsg `json:"steps,omitempty"`
	RawOutput string            `json:"raw_output,omitempty"`
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
