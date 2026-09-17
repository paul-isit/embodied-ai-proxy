package pipeline

import (
	"embodied-ai-proxy/backend/internal/rosbridge"
	"strings"
	"testing"
)

func TestFormatTableBounds_Available(t *testing.T) {
	bounds := rosbridge.TableBounds{Available: true, XMin: -0.6, XMax: 0.6, YMin: -0.4, YMax: 0.4}
	got := formatTableBounds(bounds)

	for _, want := range []string{"-0.60", "0.60", "-0.40", "0.40"} {
		if !strings.Contains(got, want) {
			t.Errorf("formatTableBounds() = %q, want it to contain %q", got, want)
		}
	}
}

func TestFormatTableBounds_Unavailable(t *testing.T) {
	got := formatTableBounds(rosbridge.TableBounds{})
	if !strings.Contains(got, "No table boundary information") {
		t.Errorf("formatTableBounds() = %q, want the unavailable fallback message", got)
	}
}

func TestBuildPrompt_SubstitutesTableBounds(t *testing.T) {
	p := &Pipeline{systemPrompt: testSystemPrompt, schemaBlock: "{}"}

	got := p.buildPrompt("go home", nil, nil, nil, rosbridge.TableBounds{Available: true, XMin: -0.6, XMax: 0.6, YMin: -0.4, YMax: 0.4})

	if strings.Contains(got, "{table_bounds}") {
		t.Errorf("buildPrompt() left the {table_bounds} placeholder unsubstituted: %q", got)
	}
	if !strings.Contains(got, "-0.60") {
		t.Errorf("buildPrompt() = %q, want it to contain the table bounds", got)
	}
}
