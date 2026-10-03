package pipeline

import (
	"context"
	"embodied-ai-proxy/backend/internal/matcher"
	"embodied-ai-proxy/backend/internal/rosbridge"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
)

// quotedTerm pulls the first quoted substring out of a message - the system
// prompt's own "graceful abort" examples (see Example 11) consistently
// single-quote the exact object name the LLM couldn't resolve, e.g.
// "Target object 'blu cube' was not found...". Double quotes are matched
// too as a fallback: message text is still LLM-authored free text (the
// schema only requires a non-empty string), so a refusal that doesn't
// follow the taught convention is possible - in that case this simply
// fails to match and recoverMissingObject falls through to the LLM's
// own refusal unchanged, rather than crashing or retrying incorrectly.
var quotedTerm = regexp.MustCompile(`'([^']+)'|"([^"]+)"`)

// missingObjectCorrectionTemplate is appended to the original prompt for a
// single retry attempt, not baked into the static system prompt - it only
// exists for the one request that needs it.
const missingObjectCorrectionTemplate = `

## Correction
Your previous response refused this command, reporting that '%s' did not match a registered object. '%s' should actually be understood as the registered object '%s'. Regenerate the full recipe now, using '%s' as the exact object name wherever '%s' was intended, and follow all the same instructions as before.`

// recoverMissingObject handles a "missing_object" refusal the LLM produced
// on its own: unlike a "success" recipe with a typo'd name (which
// resolveRecipeNames can just clean up in place), a bare error response
// carries no recipe structure at all, so there is nothing to patch - the
// only way to recover the lost action/parameters is to ask the LLM to
// regenerate the whole thing, now that the ambiguous name has been resolved.
//
// The flagged name is matched deterministically against the live object
// list first, the same way resolveRecipeNames would:
//   - exactly one candidate: retry once with a correction hint appended to
//     the original prompt, and use whatever that retry produces.
//   - two or more candidates: this is genuine ambiguity a retry can't fix
//     (only the user can say which one) - return our own "please specify"
//     error built from the matcher's candidates, without calling the LLM
//     again.
//   - no candidates: nothing productive to do; fall through to the LLM's
//     own refusal unchanged.
func (p *Pipeline) recoverMissingObject(ctx context.Context, userText, originalPrompt, rawOutput string, doc any, environment rosbridge.EnvironmentParams) (Result, bool) {
	match := quotedTerm.FindStringSubmatch(recipeErrorMessage(doc))
	if match == nil {
		return Result{}, false
	}
	// Exactly one alternative matched - single-quoted (group 1) or
	// double-quoted (group 2) - the other is always empty.
	flagged := match[1]
	if flagged == "" {
		flagged = match[2]
	}

	outcome := matcher.Resolve(flagged, environment.Objects)
	switch {
	case outcome.Matched:
		return p.retryWithResolvedName(ctx, userText, originalPrompt, flagged, outcome.Resolved, environment)
	case outcome.Ambiguous:
		log.Printf("[Pipeline] command %q: missing_object refusal for %q is ambiguous (%v); asking user to specify instead of retrying",
			userText, flagged, outcome.Candidates)
		errDoc, raw := buildNameError("invalid_command", fmt.Sprintf(
			"Execution aborted. '%s' matches more than one object (%s), and the command doesn't say which. Please specify which one.",
			flagged, strings.Join(outcome.Candidates, ", "),
		))
		return Result{RawOutput: rawOutput, Parsed: json.RawMessage(raw), Doc: errDoc}, true
	default:
		return Result{}, false
	}
}

func (p *Pipeline) retryWithResolvedName(ctx context.Context, userText, originalPrompt, flagged, resolved string, environment rosbridge.EnvironmentParams) (Result, bool) {
	log.Printf("[Pipeline] command %q: missing_object refusal for %q resolves to %q; retrying once", userText, flagged, resolved)

	retryPrompt := originalPrompt + fmt.Sprintf(missingObjectCorrectionTemplate, flagged, flagged, resolved, resolved, flagged)

	rawOutput, err := p.callLLMProxy(ctx, retryPrompt)
	if err != nil {
		log.Printf("[Pipeline] command %q: retry LLM call failed: %v", userText, err)
		return Result{}, false
	}

	candidate := extractJSON(rawOutput)
	doc, err := p.validator.Validate([]byte(candidate))
	if err != nil {
		log.Printf("[Pipeline] command %q: retry produced an invalid recipe: %v; raw output: %s", userText, err, rawOutput)
		return Result{}, false
	}

	log.Printf("[Pipeline] command %q: retry produced: %s", userText, candidate)

	if recipeStatus(doc) == "success" {
		return p.finalizeSuccess(userText, rawOutput, doc, environment), true
	}
	return Result{RawOutput: rawOutput, Parsed: json.RawMessage(candidate), Doc: doc}, true
}
