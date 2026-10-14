// Package eval runs JSON test suites of prompts through the pipeline and
// checks each result against the expected outcomes, without needing a robot.
package eval

import (
	"bytes"
	"context"
	"embodied-ai-proxy/backend/internal/pipeline"
	"embodied-ai-proxy/backend/internal/rosbridge"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// environment stands in for the robot's live environment.
type environment struct {
	Objects      []string              `json:"objects"`
	Movements    []string              `json:"movements"`
	Orientations []string              `json:"orientations"`
	TableBounds  rosbridge.TableBounds `json:"table_bounds"`
}

// caseSpec is one test case as written in a suite file.
type caseSpec struct {
	Name   string `json:"name"`
	Prompt string `json:"prompt"`
	// Environment overrides the suite's environment for this case only.
	Environment *environment `json:"environment"`
	// Expect lists acceptable outcomes; the case passes if any one matches.
	// Empty means only "the pipeline produced a schema-valid result".
	Expect []map[string]any `json:"expect"`
}

// suiteFile is the parsed contents of one suite JSON file.
type suiteFile struct {
	Environment environment `json:"environment"`
	Cases       []caseSpec  `json:"cases"`
}

// Run runs every suite through promptPipeline, writes a report to out and
// returns the number of failed cases. A suite that can't be loaded counts as
// one failure.
func Run(ctx context.Context, promptPipeline *pipeline.Pipeline, model string, suitePaths []string, out io.Writer) int {
	start := time.Now()
	casesRun := 0
	var failures []string

	fmt.Fprintf(out, "\nEvaluating %s\n", model)
	for _, path := range suitePaths {
		suiteLabel := displayPath(path)
		suite, err := load(path)
		if err != nil {
			fmt.Fprintf(out, "\nFAIL %s\n    %s\n", suiteLabel, err)
			failures = append(failures, suiteLabel+"  could not load")
			continue
		}
		fmt.Fprintf(out, "\n%s · %d cases\n\n", suiteLabel, len(suite.Cases))

		nameWidth := 0
		for i, tc := range suite.Cases {
			nameWidth = max(nameWidth, len(caseName(tc, i)))
		}
		for i, tc := range suite.Cases {
			if ctx.Err() != nil {
				fmt.Fprintf(out, "\naborted: %v\n", ctx.Err())
				return len(failures) + 1
			}
			name := caseName(tc, i)
			env := suite.Environment
			if tc.Environment != nil {
				env = *tc.Environment
			}

			caseStart := time.Now()
			result := promptPipeline.Run(ctx, tc.Prompt, rosbridge.EnvironmentParams{
				Objects:      env.Objects,
				Movements:    env.Movements,
				Orientations: env.Orientations,
				TableBounds:  env.TableBounds,
			})
			casesRun++

			mismatches, passed := check(tc, result)
			status := "PASS"
			if !passed {
				status = "FAIL"
			}
			fmt.Fprintf(out, "  %s %-*s  %.1fs\n", status, nameWidth, name, time.Since(caseStart).Seconds())
			if passed {
				continue
			}
			if len(suitePaths) > 1 {
				name += "  " + suiteLabel
			}
			failures = append(failures, name)
			field(out, "prompt", tc.Prompt)
			if result.Error != "" {
				field(out, "error", result.Error)
				if result.RawOutput != "" {
					field(out, "raw", truncate(compact(result.RawOutput), 400))
				}
			} else {
				for j, want := range tc.Expect {
					label := "want"
					if j > 0 {
						label = "or"
					}
					field(out, label, renderRecipe(want))
					field(out, "", "↳ "+mismatches[j])
				}
				field(out, "got", renderRecipe(result.Doc))
			}
			fmt.Fprintln(out)
		}
	}

	failed := len(failures)
	summary := fmt.Sprintf("%d passed", casesRun-failed)
	if failed > 0 {
		summary += fmt.Sprintf(" · %d failed", failed)
	}
	fmt.Fprintf(out, "\n%s\n  %s · %d cases · %.1fs\n", strings.Repeat("─", 48), summary, casesRun, time.Since(start).Seconds())
	for _, failure := range failures {
		fmt.Fprintf(out, "  FAIL %s\n", failure)
	}
	fmt.Fprintln(out)
	return failed
}

func caseName(tc caseSpec, i int) string {
	if tc.Name != "" {
		return tc.Name
	}
	return fmt.Sprintf("case %d", i+1)
}

func load(path string) (suiteFile, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return suiteFile{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()             // numbers stay json.Number, like in pipeline results
	decoder.DisallowUnknownFields() // catch typos like "expected"
	var suite suiteFile
	if err := decoder.Decode(&suite); err != nil {
		return suiteFile{}, fmt.Errorf("parse %s: %w", path, err)
	}
	for i, tc := range suite.Cases {
		if tc.Prompt == "" {
			return suiteFile{}, fmt.Errorf("%s: case %d has no prompt", path, i+1)
		}
	}
	return suite, nil
}

// check reports whether result satisfies tc and, if not, why each
// expected outcome didn't match.
func check(tc caseSpec, result pipeline.Result) (mismatches []string, passed bool) {
	if result.Error != "" {
		return nil, false
	}
	if len(tc.Expect) == 0 {
		return nil, true
	}
	for _, want := range tc.Expect {
		mismatch := match("", want, result.Doc)
		if mismatch == "" {
			return nil, true
		}
		mismatches = append(mismatches, mismatch)
	}
	return mismatches, false
}

// match returns where got first differs from want, or "" if it matches.
// Keys missing from want are ignored, a null in want means the key must be
// absent, and arrays must match item by item.
func match(path string, want, got any) string {
	switch typedWant := want.(type) {
	case map[string]any:
		gotObject, ok := got.(map[string]any)
		if !ok {
			return fmt.Sprintf("%s: want an object, got %s", orRoot(path), describe(got))
		}
		keys := make([]string, 0, len(typedWant))
		for key := range typedWant {
			keys = append(keys, key)
		}
		slices.Sort(keys) // same first mismatch every run
		for _, key := range keys {
			wantValue := typedWant[key]
			gotValue, present := gotObject[key]
			switch {
			case wantValue == nil && present:
				return fmt.Sprintf("%s: want absent, got %s", orRoot(path+"."+key), describe(gotValue))
			case wantValue == nil:
			case !present:
				return fmt.Sprintf("%s: missing, want %s", orRoot(path+"."+key), describe(wantValue))
			default:
				if mismatch := match(path+"."+key, wantValue, gotValue); mismatch != "" {
					return mismatch
				}
			}
		}
		return ""
	case []any:
		gotArray, ok := got.([]any)
		if !ok {
			return fmt.Sprintf("%s: want an array, got %s", orRoot(path), describe(got))
		}
		if len(gotArray) != len(typedWant) {
			return fmt.Sprintf("%s: want %d items, got %d", orRoot(path), len(typedWant), len(gotArray))
		}
		for i := range typedWant {
			if mismatch := match(fmt.Sprintf("%s[%d]", path, i), typedWant[i], gotArray[i]); mismatch != "" {
				return mismatch
			}
		}
		return ""
	case json.Number:
		if gotNumber, ok := got.(json.Number); ok {
			wantFloat, wantErr := typedWant.Float64()
			gotFloat, gotErr := gotNumber.Float64()
			if wantErr == nil && gotErr == nil && wantFloat == gotFloat {
				return ""
			}
		}
	default:
		if want == got {
			return ""
		}
	}
	return fmt.Sprintf("%s: want %s, got %s", orRoot(path), describe(want), describe(got))
}

func orRoot(path string) string {
	if path == "" {
		return "(root)"
	}
	return strings.TrimPrefix(path, ".")
}

func describe(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(encoded)
}

func compact(text string) string {
	var compacted bytes.Buffer
	if json.Compact(&compacted, []byte(text)) == nil {
		return compacted.String()
	}
	return strings.TrimSpace(text)
}

// renderRecipe formats a recipe or expected pattern as a step chain, e.g.
// "home → pickup(target=blue_cube)", one step per line if it's too long.
func renderRecipe(doc any) string {
	recipe, ok := doc.(map[string]any)
	if !ok {
		return describe(doc)
	}
	if recipe["status"] == "error" {
		text := "error"
		if errorType, ok := recipe["error_type"].(string); ok {
			text += " " + errorType
		}
		if message, ok := recipe["message"].(string); ok {
			text += ": " + message
		}
		return text
	}
	steps, ok := recipe["steps"].([]any)
	if !ok {
		return describe(doc)
	}
	rendered := make([]string, len(steps))
	for i, step := range steps {
		rendered[i] = renderStep(step)
	}
	if line := strings.Join(rendered, " → "); len(line) <= 80 {
		return line
	}
	for i := range rendered {
		rendered[i] = fmt.Sprintf("%d. %s", i+1, rendered[i])
	}
	return strings.Join(rendered, "\n")
}

func renderStep(step any) string {
	fields, ok := step.(map[string]any)
	if !ok {
		return describe(step)
	}
	action, _ := fields["action"].(string)
	if action == "" {
		action = "*" // expected step that matches any action
	}
	params, _ := fields["parameters"].(map[string]any)
	if len(params) == 0 {
		return action
	}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	pairs := make([]string, len(keys))
	for i, key := range keys {
		switch value := params[key].(type) {
		case nil:
			pairs[i] = key + "=<none>"
		case string:
			pairs[i] = key + "=" + value
		default:
			pairs[i] = key + "=" + describe(value)
		}
	}
	return action + "(" + strings.Join(pairs, ", ") + ")"
}

func truncate(text string, limit int) string {
	if runes := []rune(text); len(runes) > limit {
		return string(runes[:limit]) + "… (full output in data/logs/eval.log)"
	}
	return text
}

// displayPath returns path relative to the working directory when it's
// inside it.
func displayPath(path string) string {
	if cwd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(cwd, path); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return path
}

// field prints a labelled detail line, aligning any continuation lines.
func field(out io.Writer, label, value string) {
	value = strings.ReplaceAll(value, "\n", "\n"+strings.Repeat(" ", 14))
	fmt.Fprintf(out, "      %-6s  %s\n", label, value)
}
