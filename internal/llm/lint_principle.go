package llm

import (
	"fmt"

	"github.com/sdougbrown/writetighter/internal/check"
	"github.com/sdougbrown/writetighter/internal/guidance"
)

// lintPrincipleFallback maps deterministic lint rule IDs (CORE.UNEXPANDED_ABBREV
// etc.) to the revision principle whose stated direction matches the rule's
// intent.
//
// The revise prompt presents revision principles and lint rules/IDs under the
// same CORE.* namespace, so models routinely echo a lint rule ID into
// principle_ids (e.g. gemma4 returning CORE.UNEXPANDED_ABBREV). Coercing the
// echoed ID to its closest revision principle keeps the finding useful instead
// of failing the entire revision batch over one mislabeled field. Rules without
// a clear semantic match (CORE.CONTRACTION, CORE.BANNED_MODAL) are intentionally
// omitted so their echoed IDs are dropped rather than misattributed.
var lintPrincipleFallback = map[string]string{
	"CORE.SENTENCE_LENGTH":        "CORE.SHORT_SENTENCE",
	"CORE.DENSE_PARAGRAPH":        "CORE.ONE_TOPIC_PARAGRAPH",
	"CORE.NOUN_STACK":             "CORE.EXPLICIT_RELATIONSHIPS",
	"CORE.GERUND_OPENER":          "CORE.ACTIVE_DIRECT_VOICE",
	"CORE.TERM_DISCOURAGED":       "CORE.APPROVED_WORDS",
	"CORE.TERM_CASE":              "CORE.ONE_TERM_IDEA",
	"CORE.TERM_CONSISTENCY":       "CORE.ONE_TERM_IDEA",
	"CORE.TERM_UNKNOWN":           "CORE.APPROVED_WORDS",
	"CORE.LATIN_ABBREV":           "CORE.EXPLICIT_RELATIONSHIPS",
	"CORE.UNEXPANDED_ABBREV":      "CORE.EXPLICIT_RELATIONSHIPS",
	"CORE.CORPUS_NOVELTY":         "CORE.APPROVED_WORDS",
	"CORE.PROCEDURE_MULTI_ACTION": "CORE.EXPLICIT_RELATIONSHIPS",
	"CORE.TIME_ANCHOR":            "CORE.TIMELESS_PROSE",
	"CORE.GERUND_HEADING":         "CORE.ACTIVE_DIRECT_VOICE",
	"CORE.SEQUENTIAL_BULLET":      "CORE.CAUSAL_ORDER",
}

// sanitizePrincipleIDs reduces a model-supplied principle_ids list to the set
// of valid revision principles, coercing deterministic lint rule IDs that share
// the CORE.* namespace into their closest revision principle.
//
//   - a valid revision principle is kept, with duplicates rejected;
//   - a known lint rule ID is coerced to its mapped principle (or dropped when
//     it has no mapping) so an echoed lint rule never fails the batch;
//   - a genuinely unknown ID (a hallucination) is treated as a contract
//     violation and reported as an error.
//
// The returned list is de-duplicated and preserves the model's ordering.
func sanitizePrincipleIDs(ids []string) ([]string, error) {
	seen := make(map[string]bool, len(ids))     // present literally or via coercion
	seenTrue := make(map[string]bool, len(ids)) // present literally from the model
	cleaned := make([]string, 0, len(ids))
	for _, id := range ids {
		if guidance.IsPrincipleID(id) {
			if seenTrue[id] {
				return nil, fmt.Errorf("duplicate principle id: %s", id)
			}
			seenTrue[id] = true
			if !seen[id] {
				seen[id] = true
				cleaned = append(cleaned, id)
			}
			continue
		}
		// Not a revision principle. If it is a deterministic lint rule ID the
		// prompt itself surfaced, coerce or drop it gracefully; otherwise the
		// model invented an ID and the response must be rejected.
		if check.Get(id) != nil {
			if target := lintPrincipleFallback[id]; target != "" && !seen[target] {
				seen[target] = true
				cleaned = append(cleaned, target)
			}
			continue
		}
		return nil, fmt.Errorf("unknown principle id: %s", id)
	}
	return cleaned, nil
}
