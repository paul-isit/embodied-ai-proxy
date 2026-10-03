package matcher

import (
	"reflect"
	"sort"
	"testing"
)

var objects = []string{"water_bottle", "push_block", "blue_cube", "delivery_tray"}

func TestResolve_ExactMatch(t *testing.T) {
	out := Resolve("blue_cube", objects)
	if !out.Matched || out.Resolved != "blue_cube" || out.Ambiguous {
		t.Fatalf("expected exact match on blue_cube, got %+v", out)
	}
}

func TestResolve_TypoSingleCandidate(t *testing.T) {
	// The real observed failure: "blu cube" against a list with exactly one
	// cube should resolve to it, the same way the system prompt's own
	// worked example ("rad cube" -> red_cube) says it should.
	out := Resolve("blu cube", objects)
	if !out.Matched || out.Resolved != "blue_cube" || out.Ambiguous {
		t.Fatalf("expected typo resolution to blue_cube, got %+v", out)
	}
}

func TestResolve_GenericReferenceSingleCandidate(t *testing.T) {
	out := Resolve("the cube", objects)
	if !out.Matched || out.Resolved != "blue_cube" {
		t.Fatalf("expected 'the cube' to resolve to blue_cube, got %+v", out)
	}

	out = Resolve("the bottle", objects)
	if !out.Matched || out.Resolved != "water_bottle" {
		t.Fatalf("expected 'the bottle' to resolve to water_bottle, got %+v", out)
	}
}

func TestResolve_GenericReferenceAmbiguous(t *testing.T) {
	twoCubes := []string{"blue_cube", "red_cube", "delivery_tray"}
	out := Resolve("the cube", twoCubes)
	if out.Matched || !out.Ambiguous {
		t.Fatalf("expected ambiguous result for 'the cube' with two cubes, got %+v", out)
	}
	got := append([]string(nil), out.Candidates...)
	sort.Strings(got)
	want := []string{"blue_cube", "red_cube"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected candidates %v, got %v", want, got)
	}
}

func TestResolve_NoMatch(t *testing.T) {
	// The other real observed failure: a verb typo ("pivk") landing in the
	// target field shouldn't fuzzy-match any real object - it's not close to
	// any of them, and matcher can't recover a reference that was never
	// correctly extracted in the first place.
	out := Resolve("pivk", objects)
	if out.Matched || out.Ambiguous {
		t.Fatalf("expected no match for 'pivk', got %+v", out)
	}
	out = Resolve("pivk up teh bottle", objects)
	if out.Matched || out.Ambiguous {
		t.Fatalf("expected no match for the full garbled phrase, got %+v", out)
	}
}

func TestResolve_UnrelatedWordsDoNotFalselyMatch(t *testing.T) {
	for _, value := range []string{"push_block", "delivery_tray"} {
		out := Resolve("blu cube", []string{value})
		if out.Matched {
			t.Fatalf("did not expect 'blu cube' to match unrelated name %q, got %+v", value, out)
		}
	}
}

func TestResolve_CaseInsensitiveExact(t *testing.T) {
	out := Resolve("Blue_Cube", objects)
	if !out.Matched || out.Resolved != "blue_cube" {
		t.Fatalf("expected case-insensitive match to blue_cube, got %+v", out)
	}
}
