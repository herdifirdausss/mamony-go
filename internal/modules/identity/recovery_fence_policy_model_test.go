package identity

import (
	"slices"
	"testing"
)

// This toy model classifies invented observations only. It is not an
// application policy, an evidence authenticator, or a release mechanism.
func TestIdentityRecoveryFencePolicyModel(t *testing.T) {
	type observation struct {
		category string
		scope    string
		revision int
		outcome  string
	}
	type review struct {
		classification string
		disposition    string
		distinct       int
	}

	// Every observation is symbolic; no Identity or recovery state is read.
	evaluate := func(input []observation) review {
		expected := []string{"alpha", "beta", "gamma"}
		result := review{classification: "INCOMPLETE", disposition: "HOLD"}
		seen := make(map[string]observation)
		for _, item := range input {
			if !slices.Contains(expected, item.category) {
				result.classification = "UNKNOWN"
				return result
			}
			if item.scope != "toy-scope" {
				result.classification = "MISMATCH"
				return result
			}
			if item.revision != 2 {
				result.classification = "STALE"
				return result
			}
			if prior, ok := seen[item.category]; ok {
				if prior != item {
					result.classification = "CONTRADICTORY"
					return result
				}
				continue
			}
			seen[item.category] = item
			result.distinct++
		}
		for _, category := range expected {
			item, ok := seen[category]
			if !ok {
				return result
			}
			switch item.outcome {
			case "consistent":
			case "contradictory":
				result.classification = "CONTRADICTORY"
				return result
			default:
				result.classification = "UNKNOWN"
				return result
			}
		}
		result.classification = "REVIEW_COMPLETE"
		return result
	}

	alpha := observation{"alpha", "toy-scope", 2, "consistent"}
	beta := observation{"beta", "toy-scope", 2, "consistent"}
	gamma := observation{"gamma", "toy-scope", 2, "consistent"}
	complete := []observation{alpha, beta, gamma}
	cases := []struct {
		name         string
		input        []observation
		want         string
		wantDistinct int
		reorder      bool
	}{
		{"E1-01_complete", complete, "REVIEW_COMPLETE", 3, false},
		{"E1-02_empty", nil, "INCOMPLETE", 0, false},
		{"E1-03_missing", []observation{alpha, gamma}, "INCOMPLETE", 2, false},
		{"E1-04_unknown_outcome", []observation{alpha, beta, {"gamma", "toy-scope", 2, "unknown"}}, "UNKNOWN", 3, false},
		{"E1-05_contradictory", []observation{alpha, beta, {"gamma", "toy-scope", 2, "contradictory"}}, "CONTRADICTORY", 3, false},
		{"E1-06_stale_revision", []observation{alpha, beta, {"gamma", "toy-scope", 1, "consistent"}}, "STALE", 2, false},
		{"E1-07_scope_mismatch", []observation{alpha, beta, {"gamma", "other-toy-scope", 2, "consistent"}}, "MISMATCH", 2, false},
		{"E1-08_partial_review", []observation{alpha}, "INCOMPLETE", 1, false},
		{"E1-09_identical_duplicate", []observation{alpha, beta, gamma, beta}, "REVIEW_COMPLETE", 3, false},
		{"E1-10_conflicting_duplicate", []observation{alpha, beta, gamma, {"beta", "toy-scope", 2, "contradictory"}}, "CONTRADICTORY", 3, false},
		{"E1-11_unknown_category", []observation{alpha, beta, gamma, {"delta", "toy-scope", 2, "consistent"}}, "UNKNOWN", 3, false},
		{"E1-12_reordered_repeated", complete, "REVIEW_COMPLETE", 3, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := slices.Clone(tc.input)
			before := slices.Clone(input)
			assertReview := func(got review) {
				t.Helper()
				if got.classification != tc.want || got.distinct != tc.wantDistinct {
					t.Fatalf("review = %+v, want %s with %d distinct observations", got, tc.want, tc.wantDistinct)
				}
				if got.disposition != "HOLD" {
					t.Fatal("symbolic review must remain HOLD")
				}
			}
			assertReview(evaluate(input))
			assertReview(evaluate(input))
			if !slices.Equal(input, before) {
				t.Fatal("evaluation changed its input")
			}
			if tc.reorder {
				slices.Reverse(input)
				reordered := slices.Clone(input)
				assertReview(evaluate(input))
				assertReview(evaluate(input))
				if !slices.Equal(input, reordered) {
					t.Fatal("evaluation changed reordered input")
				}
			}
		})
	}
}
