# Whole-file code-comment evaluation rubric

You are judging an automated technical-writing review of comments in complete
source files. The expected material includes the source corpus and an
`expected-findings.json` file produced by human review. Treat that finding set
as the fixed, exhaustive recall denominator. Verify every reported finding
against the surrounding implementation. Do not reward volume or change the
recall denominator based on the candidate output.

A genuine finding identifies an actual comment that warrants deletion,
clarification, or rewriting because it is inaccurate, ambiguous, compressed,
stale, misleading about scope, or narrates syntax without adding useful
rationale, contract, invariant, constraint, or navigation.

Use these boundaries:

- Executable code, string literals, and Python docstrings are read-only and are
  not comment targets for this evaluation. Rust/Go/TypeScript documentation
  comments are comment targets.
- A concise section label can be useful navigation in a long function. Do not
  call it redundant merely because it is short.
- A comment explaining an invariant or hidden precondition is useful even when
  adjacent syntax partly reflects it.
- A deletion is valid only when the comment adds no useful information.
- A rewrite is safe only when every behavior, identifier, condition, and claim
  in the replacement is established by the source.
- A clarification is actionable when it asks for a specific missing fact that
  matters to the comment's correctness. Do not reward questions the code
  already answers.
- Independently verify `comment_target`; it is heuristic metadata, not ground
  truth. An unresolved target counts as a false positive unless its exact
  source comment can still be identified unambiguously.
- Driver errors or incomplete file coverage reduce quality and recall.

Score fields:

- `valid_findings`: distinct expected findings correctly identified by the
  report. Do not count optional improvements outside `expected-findings.json`.
- `false_positives`: reported findings that do not match an expected finding,
  are duplicate, ungrounded, or target prose outside actual comments.
- `missed_findings`: expected findings absent from the report. This must equal
  the number of unmatched entries in `expected-findings.json`.
- `non_comment_targets`: findings aimed at code, strings, docstrings, or other
  non-comment text.
- `actionable_findings`: valid findings with a safe deletion/replacement or a
  necessary concrete clarification question.
- `estimated_recall`: `valid_findings / (valid_findings + missed_findings)`.
  The denominator is the fixed expected finding count.
- `replacement_safety`: 0–100 assessment of whether proposed rewrites/deletions
  preserve established technical meaning. Use 100 when there are no unsafe
  replacements; do not penalize a run merely because it reports no rewrites.
- `overall_quality`: 0–100 holistic score emphasizing correctness and precision,
  then recall and actionability. A short accurate report beats a noisy one.

Return only the requested JSON score object. Do not include explanations or
additional keys.
