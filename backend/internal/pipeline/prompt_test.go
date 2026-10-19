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

	got := p.buildPrompt("go home", rosbridge.EnvironmentParams{
		TableBounds: rosbridge.TableBounds{Available: true, XMin: -0.6, XMax: 0.6, YMin: -0.4, YMax: 0.4},
	})

	if strings.Contains(got, "{table_bounds}") {
		t.Errorf("buildPrompt() left the {table_bounds} placeholder unsubstituted: %q", got)
	}
	if !strings.Contains(got, "-0.60") {
		t.Errorf("buildPrompt() = %q, want it to contain the table bounds", got)
	}
}

func TestFormatHeldObjects(t *testing.T) {
	held := []string{"water_bottle"}
	none := []string{}
	cases := []struct {
		name string
		held *[]string
		want string
	}{
		{"unknown", nil, "Unknown"},
		{"nothing held", &none, "Nothing is currently held."},
		{"holding an object", &held, "- water_bottle"},
	}
	for _, tc := range cases {
		if got := formatHeldObjects(tc.held); !strings.Contains(got, tc.want) {
			t.Errorf("%s: formatHeldObjects() = %q, want it to contain %q", tc.name, got, tc.want)
		}
	}
}

func TestBuildPrompt_SubstitutesHeldObjects(t *testing.T) {
	p := &Pipeline{systemPrompt: testSystemPrompt, schemaBlock: "{}"}
	held := []string{"water_bottle"}

	got := p.buildPrompt("put it down", rosbridge.EnvironmentParams{HeldObjects: &held})

	if strings.Contains(got, "{held_objects}") {
		t.Errorf("buildPrompt() left the {held_objects} placeholder unsubstituted: %q", got)
	}
	if !strings.Contains(got, "- water_bottle") {
		t.Errorf("buildPrompt() = %q, want it to list the held object", got)
	}
}

func TestDefaultSystemPrompt_HasHeldObjectsPlaceholder(t *testing.T) {
	if !strings.Contains(DefaultSystemPrompt, placeholderHeldObjects) {
		t.Errorf("DefaultSystemPrompt is missing the %s placeholder", placeholderHeldObjects)
	}
}
