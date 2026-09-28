// Package matcher deterministically resolves a name the LLM put in a recipe
// (an object/movement/orientation reference) against the live list of
// registered names for this session, so the same typo/partial-reference
// tolerance the system prompt tries to teach the LLM ("blu cube" -> the only
// cube available, "the cube" -> the only cube available, but "the cube" with
// two cubes present -> ask which one) is enforced consistently regardless of
// which LLM produced the recipe, rather than relying on the model to get it
// right every time.
package matcher

import "strings"

// fuzzyThreshold is the minimum normalized similarity (1 - editDistance/len)
// for two normalized strings to be considered a fuzzy (typo) match. Tuned
// against real observed cases: "blu cube"/"blue cube" (~0.89) and
// "rad cube"/"red cube" (~0.88) both clear it comfortably, while an
// unrelated short word like "pivk" against any real object name scores well
// below it.
const fuzzyThreshold = 0.7

// stopWords are filtered out of a value before token-containment matching,
// so a generic reference like "the cube" or "pick up the bottle" resolves on
// its meaningful word ("cube", "bottle") rather than being rejected for
// carrying filler words the registered name doesn't contain.
var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "it": true, "its": true,
	"this": true, "that": true, "up": true, "to": true, "please": true,
	"one": true,
}

// Outcome is the result of resolving a single name against a candidate list.
type Outcome struct {
	// Resolved is the canonical registered name to use, set only when
	// exactly one candidate matched.
	Resolved string
	// Matched is true when exactly one candidate matched - the common,
	// unambiguous case (including an already-exact match).
	Matched bool
	// Ambiguous is true when two or more candidates matched equally well -
	// callers should ask the user to specify rather than guessing.
	Ambiguous bool
	// Candidates lists the registered names that matched, for building an
	// error message when Ambiguous is true (or empty when nothing matched).
	Candidates []string
}

// Resolve checks value (whatever the LLM put in a target/destination/vector/
// orientation field) against the registered names list, in three steps:
// exact match short-circuits immediately; otherwise both fuzzy (typo) and
// token-containment (partial/generic reference) matching are tried, and any
// registered name either technique agrees on becomes a candidate. Exactly
// one candidate resolves automatically; zero or several do not.
func Resolve(value string, registered []string) Outcome {
	for _, name := range registered {
		if value == name {
			return Outcome{Resolved: name, Matched: true, Candidates: []string{name}}
		}
	}

	normalizedValue := normalize(value)
	valueTokens := tokenize(normalizedValue)

	var candidates []string
	for _, name := range registered {
		normalizedName := normalize(name)
		if similarity(normalizedValue, normalizedName) >= fuzzyThreshold || tokensContained(valueTokens, tokenize(normalizedName)) {
			candidates = append(candidates, name)
		}
	}

	switch len(candidates) {
	case 0:
		return Outcome{}
	case 1:
		return Outcome{Resolved: candidates[0], Matched: true, Candidates: candidates}
	default:
		return Outcome{Ambiguous: true, Candidates: candidates}
	}
}

// normalize lowercases value and collapses any run of non-alphanumeric
// characters (spaces, underscores, punctuation) into a single space, so
// "blue_cube" and "blu cube" compare on equal footing.
func normalize(value string) string {
	var b strings.Builder
	lastWasSpace := true // trims a leading separator without a special case
	for _, r := range strings.ToLower(value) {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlnum {
			b.WriteRune(r)
			lastWasSpace = false
		} else if !lastWasSpace {
			b.WriteRune(' ')
			lastWasSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func tokenize(normalized string) []string {
	if normalized == "" {
		return nil
	}
	var tokens []string
	for _, word := range strings.Split(normalized, " ") {
		if word != "" && !stopWords[word] {
			tokens = append(tokens, word)
		}
	}
	return tokens
}

// tokensContained reports whether every meaningful word in valueTokens
// appears among nameTokens - e.g. "cube" (from "the cube") is contained in
// ("blue", "cube"). Empty valueTokens (a value that was pure filler, or
// empty) never matches anything this way.
func tokensContained(valueTokens, nameTokens []string) bool {
	if len(valueTokens) == 0 {
		return false
	}
	nameSet := make(map[string]bool, len(nameTokens))
	for _, t := range nameTokens {
		nameSet[t] = true
	}
	for _, t := range valueTokens {
		if !nameSet[t] {
			return false
		}
	}
	return true
}

// similarity returns 1 - (edit distance / longer length), i.e. 1.0 for an
// exact match and closer to 0.0 the more the two strings differ.
func similarity(a, b string) float64 {
	if a == "" && b == "" {
		return 1
	}
	longer := len(a)
	if len(b) > longer {
		longer = len(b)
	}
	if longer == 0 {
		return 1
	}
	return 1 - float64(levenshtein(a, b))/float64(longer)
}

// levenshtein computes the classic edit distance between a and b via a
// single-row dynamic-programming pass.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	curr := make([]int, len(rb)+1)
	for i := 1; i <= len(ra); i++ {
		curr[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			deletion := prev[j] + 1
			insertion := curr[j-1] + 1
			substitution := prev[j-1] + cost
			curr[j] = min3(deletion, insertion, substitution)
		}
		prev, curr = curr, prev
	}
	return prev[len(rb)]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
