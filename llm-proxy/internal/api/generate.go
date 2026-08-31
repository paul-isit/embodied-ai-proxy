package api

import (
	"embodied-ai-proxy/llm-proxy/internal/provider"
	"embodied-ai-proxy/llm-proxy/internal/router"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

// maxRequestBodyBytes caps how much of an incoming /generate request body
// this reads into memory - generous for any legitimate prompt, just bounds
// a misbehaving caller from handing this an unbounded body.
const maxRequestBodyBytes = 1 << 20 // 1 MiB

type generateRequestPayload struct {
	Prompt            string   `json:"prompt"`
	SystemInstruction string   `json:"system_instruction,omitempty"`
	MaxTokens         *int     `json:"max_tokens,omitempty"`
	Temperature       *float64 `json:"temperature,omitempty"`
}

type generateResponsePayload struct {
	Text         string `json:"text,omitempty"`
	FinishReason string `json:"finish_reason,omitempty"`
	Error        string `json:"error,omitempty"`
}

// GenerateHandler exposes rt over HTTP as POST /generate - the network
// boundary the Go backend dispatches prompts across (see design.md's
// "HTTP / gRPC Internal Dispatch" between backend and LLM proxy).
func GenerateHandler(rt *router.Router) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(generateResponsePayload{Error: "method not allowed"})
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

		var payload generateRequestPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(generateResponsePayload{Error: "invalid request body: " + err.Error()})
			return
		}
		if payload.Prompt == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(generateResponsePayload{Error: "prompt is required"})
			return
		}

		log.Printf("[LLMProxy] generate: request received (prompt=%d bytes, system_instruction=%d bytes)", len(payload.Prompt), len(payload.SystemInstruction))
		start := time.Now()

		resp, err := rt.Generate(r.Context(), provider.Request{
			Prompt:            payload.Prompt,
			SystemInstruction: payload.SystemInstruction,
			MaxTokens:         payload.MaxTokens,
			Temperature:       payload.Temperature,
		})
		elapsed := time.Since(start)
		if err != nil {
			// The full error (which may include a raw, potentially verbose
			// or vendor-specific upstream response body via
			// *provider.StatusError) is logged here for debugging, but
			// never returned to the caller as-is - it's replaced with a
			// short, generic message so nothing upstream-specific leaks
			// into the backend's logs/TUI display.
			log.Printf("[LLMProxy] generate: failed after %s: %v", elapsed, err)

			status := http.StatusBadGateway
			message := "upstream provider request failed"
			var timeoutErr *provider.TimeoutError
			var statusErr *provider.StatusError
			switch {
			case errors.As(err, &timeoutErr):
				status = http.StatusGatewayTimeout
				message = "upstream provider request timed out"
			case errors.As(err, &statusErr):
				message = fmt.Sprintf("upstream provider returned an error (status %d)", statusErr.StatusCode)
			}
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(generateResponsePayload{Error: message})
			return
		}

		log.Printf("[LLMProxy] generate: succeeded in %s (response=%d bytes, finish_reason=%q)", elapsed, len(resp.Text), resp.FinishReason)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(generateResponsePayload{
			Text:         resp.Text,
			FinishReason: resp.FinishReason,
		})
	}
}
