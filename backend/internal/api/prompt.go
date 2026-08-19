package api

import (
	"embodied-ai-proxy/backend/internal/prompt"
	"encoding/json"
	"net/http"
)

type promptRequestPayload struct {
	Prompt           string   `json:"prompt"`
	AvailableObjects []string `json:"available_objects"`
}

// PromptHandler exposes the prompt pipeline over HTTP as POST /api/prompt,
// for batch evaluation (evaluate_proxy.py) and other HTTP-based queries that
// don't need a persistent WebSocket connection. Response shape mirrors the
// original Python LLMProxy.generate() return value: {raw_output, parsed}.
func PromptHandler(pipeline *prompt.Pipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(prompt.Result{Error: "method not allowed"})
			return
		}

		var payload promptRequestPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(prompt.Result{Error: "invalid request body: " + err.Error()})
			return
		}
		if payload.Prompt == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(prompt.Result{Error: "prompt is required"})
			return
		}

		result := pipeline.Run(r.Context(), payload.Prompt, payload.AvailableObjects)
		if result.Error != "" && result.RawOutput == "" {
			w.WriteHeader(http.StatusBadGateway) // transport/upstream failure, not a validation failure
		} else {
			w.WriteHeader(http.StatusOK)
		}
		json.NewEncoder(w).Encode(result)
	}
}
