package provider

import (
	"context"
	"fmt"
)

// Request is the provider-agnostic generation request the LLM proxy
// translates into each upstream provider's own API format.
type Request struct {
	Prompt            string
	SystemInstruction string
	MaxTokens         int
	Temperature       float64
}

// Response is the provider-agnostic result returned to the Go backend.
type Response struct {
	Text         string
	FinishReason string
}

// Provider is implemented by each upstream LLM adapter (Ollama, OpenAI,
// Gemini, Anthropic).
type Provider interface {
	Generate(ctx context.Context, req Request) (Response, error)
}

// TimeoutError wraps an upstream provider timeout so callers can detect it
// with errors.As instead of string-matching.
type TimeoutError struct {
	Provider string
	Cause    error
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("provider %s: request timed out: %v", e.Provider, e.Cause)
}

func (e *TimeoutError) Unwrap() error {
	return e.Cause
}
