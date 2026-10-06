package api

import (
	"context"
	"embodied-ai-proxy/backend/internal/pipeline"
	"embodied-ai-proxy/backend/internal/rosbridge"
	"embodied-ai-proxy/backend/internal/websocket"
	sharedconfig "embodied-ai-proxy/shared/config"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"time"
)

type infoResponse struct {
	Server           sharedconfig.ServerConfig `json:"server"`
	LLM              sharedconfig.LLMConfig    `json:"llm"`
	BridgeConnected  bool                      `json:"bridge_connected"`
	ClientsConnected int                       `json:"clients_connected"`
	SystemPrompt     string                    `json:"system_prompt"`
}

// InfoHandler exposes GET /api/info: a single snapshot combining the
// backend's own config, the LLM proxy's config, live hub stats, and the raw
// system prompt template. Both configs come from the one shared
// data/config/config.json, so no cross-service call to the LLM proxy is
// needed. Powers the TUI's System Info / LLM Info / Copy-System-Prompt
// features.
func InfoHandler(cfg *sharedconfig.AppConfig, hub *websocket.Hub, p *pipeline.Pipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clients, bridgeConnected := hub.Stats()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(infoResponse{
			Server:           cfg.Server,
			LLM:              cfg.Proxy.LLMConfig,
			BridgeConnected:  bridgeConnected,
			ClientsConnected: clients,
			SystemPrompt:     p.SystemPrompt(),
		})
	}
}

// resetter is the subset of rosbridge.Client's behaviour ResetHandler
// needs - a small interface so tests can fake it without a real rosbridge
// connection.
type resetter interface {
	ResetEnvironment(ctx context.Context) (string, error)
}

type resetResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// ResetHandler exposes POST /api/reset: triggers the middleware's
// /reset_environment service (reload objects/obstacles from their config
// files, clear held-object tracking), for the TUI's /reset-env command.
func ResetHandler(rb resetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(resetResponse{Message: "method not allowed"})
			return
		}

		message, err := rb.ResetEnvironment(r.Context())
		if err != nil {
			json.NewEncoder(w).Encode(resetResponse{Success: false, Message: err.Error()})
			return
		}
		json.NewEncoder(w).Encode(resetResponse{Success: true, Message: message})
	}
}

// scanTimeout covers the camera snapshot plus reading the object list before and after
const scanTimeout = 30 * time.Second

// scanner is the subset of rosbridge.Client ScanHandler needs, so tests can fake it.
type scanner interface {
	RefreshEnvironmentParams(ctx context.Context) (rosbridge.EnvironmentParams, error)
	Snapshot(ctx context.Context) (string, error)
}

type scanResponse struct {
	Success   bool     `json:"success"`
	Message   string   `json:"message"`
	Added     []string `json:"added"`
	Removed   []string `json:"removed"`
	Unchanged int      `json:"unchanged"`
}

// ScanHandler exposes POST /api/scan for the TUI's /scan command: takes a camera snapshot
// and reports which objects were added or removed.
func ScanHandler(rb scanner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(scanResponse{Message: "method not allowed"})
			return
		}
		fail := func(err error) { json.NewEncoder(w).Encode(scanResponse{Message: err.Error()}) }

		ctx, cancel := context.WithTimeout(r.Context(), scanTimeout)
		defer cancel()

		// Added/removed come from comparing object names before and after. Having Snapshot.srv
		// return the added/updated/removed lists it already gets from /update_scene_objects
		// would be exact, and would also say which objects moved.
		before, err := rb.RefreshEnvironmentParams(ctx)
		if err != nil {
			fail(fmt.Errorf("couldn't read the objects before scanning: %w", err))
			return
		}
		message, err := rb.Snapshot(ctx)
		if err != nil {
			fail(err)
			return
		}
		after, err := rb.RefreshEnvironmentParams(ctx)
		if err != nil {
			fail(fmt.Errorf("scanned, but couldn't read the objects after: %w", err))
			return
		}

		added, removed := missingFrom(after.Objects, before.Objects), missingFrom(before.Objects, after.Objects)
		json.NewEncoder(w).Encode(scanResponse{
			Success:   true,
			Message:   message,
			Added:     added,
			Removed:   removed,
			Unchanged: len(after.Objects) - len(added),
		})
	}
}

// missingFrom returns the names in a that aren't in b
func missingFrom(a, b []string) []string {
	out := []string{}
	for _, name := range a {
		if !slices.Contains(b, name) {
			out = append(out, name)
		}
	}
	return out
}

type promptRequestPayload struct {
	Prompt                string                `json:"prompt"`
	AvailableObjects      []string              `json:"available_objects"`
	AvailableMovements    []string              `json:"available_movements"`
	AvailableOrientations []string              `json:"available_orientations"`
	TableBounds           rosbridge.TableBounds `json:"table_bounds"`
}

// PromptHandler exposes the prompt pipeline over HTTP as POST /api/prompt,
// for batch evaluation (evaluate_proxy.py) and other HTTP-based queries that
// don't need a persistent WebSocket connection. Response shape mirrors the
// original Python LLMProxy.generate() return value: {raw_output, parsed}.
func PromptHandler(p *pipeline.Pipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(pipeline.Result{Error: "method not allowed"})
			return
		}

		var payload promptRequestPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(pipeline.Result{Error: "invalid request body: " + err.Error()})
			return
		}
		if payload.Prompt == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(pipeline.Result{Error: "prompt is required"})
			return
		}

		result := p.Run(r.Context(), payload.Prompt, rosbridge.EnvironmentParams{
			Objects:      payload.AvailableObjects,
			Movements:    payload.AvailableMovements,
			Orientations: payload.AvailableOrientations,
			TableBounds:  payload.TableBounds,
		})
		if result.Error != "" && result.RawOutput == "" {
			w.WriteHeader(http.StatusBadGateway) // transport/upstream failure, not a validation failure
		} else {
			w.WriteHeader(http.StatusOK)
		}
		json.NewEncoder(w).Encode(result)
	}
}
