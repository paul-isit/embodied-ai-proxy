package pipeline

import (
	"embodied-ai-proxy/backend/internal/matcher"
	"embodied-ai-proxy/backend/internal/rosbridge"
	"encoding/json"
	"fmt"
	"strings"
)

// nameField describes one recipe-step parameter that names a live,
// registered thing (an object, a relative-movement vector, or an
// orientation preset), and which of the environment's lists it must
// resolve against.
type nameField struct {
	key  string
	list []string
	kind string // used only for the error message, e.g. "object"
}

// resolveRecipeNames walks every step's parameters in a schema-valid
// "success" recipe and deterministically resolves 'target'/'destination'
// (against Objects), 'vector' (against Movements), and 'orientation'
// (against Orientations) - the same typo/partial-reference tolerance the
// system prompt already tries to teach the LLM (see the "best-guess through
// typos"/ambiguity rules and worked Examples 11-13), enforced consistently
// instead of depending on the model getting it right every time.
//
// On success, doc is mutated in place (a resolved-but-not-exact value like
// "blu cube" is rewritten to "blue_cube") and re-marshaled. If any field
// can't be resolved to exactly one candidate, the whole recipe is rejected
// with the same error shape the LLM itself would produce for a missing or
// ambiguous object (status/error_type/message), so callers don't need to
// treat this differently from an LLM-authored refusal.
//
// doc is expected to have already passed schema validation as a "success"
// recipe - the schema guarantees 'steps' is an array, every step has a
// 'parameters' object, and target/destination/vector/orientation are each
// either absent (legitimately optional) or a non-empty string. The checks
// below enforce that expectation explicitly (returning an error rather than
// silently skipping) so a violation - e.g. a future caller passing in an
// unvalidated doc - fails loudly instead of quietly resolving nothing.
func resolveRecipeNames(doc any, environment rosbridge.EnvironmentParams) (any, []byte, error) {
	root, ok := doc.(map[string]any)
	if !ok {
		return doc, nil, fmt.Errorf("recipe document is not a JSON object")
	}
	steps, ok := root["steps"].([]any)
	if !ok {
		return doc, nil, fmt.Errorf("recipe 'steps' is missing or not an array (expected a schema-validated recipe)")
	}

	for i, rawStep := range steps {
		step, ok := rawStep.(map[string]any)
		if !ok {
			return doc, nil, fmt.Errorf("step %d is not a JSON object (expected a schema-validated recipe)", i+1)
		}
		params, ok := step["parameters"].(map[string]any)
		if !ok {
			return doc, nil, fmt.Errorf("step %d 'parameters' is missing or not an object (expected a schema-validated recipe)", i+1)
		}

		for _, field := range []nameField{
			{"target", environment.Objects, "object"},
			{"destination", environment.Objects, "object"},
			{"vector", environment.Movements, "movement"},
			{"orientation", environment.Orientations, "orientation"},
		} {
			raw, present := params[field.key]
			if !present {
				continue // optional field omitted - not an error, e.g. dropoff's held-object fallback
			}
			value, ok := raw.(string)
			if !ok {
				return doc, nil, fmt.Errorf("step %d parameter %q is not a string (expected a schema-validated recipe)", i+1, field.key)
			}
			if value == "" {
				continue // schema enforces minLength 1 when present; stay defensive rather than resolve garbage
			}

			outcome := matcher.Resolve(value, field.list)
			switch {
			case outcome.Matched:
				params[field.key] = outcome.Resolved
			case outcome.Ambiguous:
				errDoc, raw := buildNameError("invalid_command", fmt.Sprintf(
					"Execution aborted. '%s' matches more than one %s (%s), and the command doesn't say which. Please specify which one.",
					value, field.kind, strings.Join(outcome.Candidates, ", "),
				))
				return errDoc, raw, nil
			default:
				errDoc, raw := buildNameError("missing_object", fmt.Sprintf(
					"Execution aborted. Referenced %s '%s' was not found in the environment map. Available: %s.",
					field.kind, value, strings.Join(field.list, ", "),
				))
				return errDoc, raw, nil
			}
		}
	}

	raw, err := json.Marshal(root)
	if err != nil {
		return doc, nil, fmt.Errorf("re-encode resolved recipe: %w", err)
	}
	return root, raw, nil
}

func buildNameError(errorType, message string) (map[string]any, []byte) {
	doc := map[string]any{
		"status":     "error",
		"error_type": errorType,
		"message":    message,
	}
	raw, _ := json.Marshal(doc)
	return doc, raw
}
