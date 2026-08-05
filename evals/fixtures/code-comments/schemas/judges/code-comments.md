# Whole-file code-comment evaluation rubric

You are judging an automated technical-writing review of comments in complete
source files. The expected material includes the supplied source corpus and an
`expected-findings.json` file produced by human review. Treat that finding set
as a fixed recall reference set, not an exhaustive list of every valid finding.
Do not change the recall denominator based on the candidate output.

Verify each reported finding against the supplied source. A model's general
knowledge can make a concern plausible, but it is not evidence of this
repository's intent. When a finding depends on callers, tests, conventions,
definitions, or files that were not supplied, classify it as context-dependent
rather than valid or invalid unless the supplied source already resolves it.

A genuine finding identifies an actual comment with a material defect: the
current wording would cause a reasonable maintainer to misunderstand behavior,
scope, contract, precedence, or rationale; preserves stale or false history; or
obscures a necessary relationship enough to impede correct maintenance. A
replacement being somewhat clearer, more explicit, or more detailed is not by
itself evidence that the existing comment is defective.

Use these boundaries:

- Executable code, string literals, and Python docstrings are read-only and are
  not comment targets for this evaluation. Rust/Go/TypeScript documentation
  comments are comment targets.
- A concise section label can be useful navigation in a long function. Rewriting
  `// Conflicts`, `// Free time`, `// Focus blocks`, or a similar accurate label
  to narrate the immediately adjacent code is optional polish and therefore
  invalid for this evaluation.
- A comment explaining an invariant or hidden precondition is useful even when
  adjacent syntax partly reflects it. Do not rewrite a correct concise ordering
  invariant merely to add `because`, repeat variable names, or restate the same
  comparison.
- Arithmetic comments in tests are valid when they concisely explain expected
  values. Rephrasing notation such as `2 blocks × 2h = 4h` into a sentence is
  optional polish unless the calculation is wrong or genuinely ambiguous.
- An accurate API comment or control-flow summary is not defective merely
  because a longer replacement could enumerate branches or implementation
  steps. Treat such expansion as invalid optional polish.
- A truthful TODO or placeholder marker is context-dependent when deciding
  whether it is stale requires an external contract or implementation plan.
- A deletion is valid only when the comment adds no useful information.
- A rewrite is safe only when every behavior, identifier, condition, and claim
  in the replacement is established by the supplied source.
- A clarification is actionable when it asks for a specific missing fact that
  matters to the comment's correctness. Do not reward questions the supplied
  code already answers.
- Independently verify `comment_target`; it is heuristic metadata, not ground
  truth. An unresolved target is invalid unless its exact source comment can
  still be identified unambiguously.
- Driver errors or incomplete file coverage reduce quality and recall.

Classify every reported finding into exactly one category:

1. **Matched expected** — it correctly identifies one distinct entry in
   `expected-findings.json`.
2. **Valid additional** — it identifies a material, locally supported defect
   that is not in the reference set. The supplied source must establish why the
   current comment is defective, not merely why the proposal is nicer. Optional
   polish, expansion, stylistic preference, and narration of adjacent code do
   not qualify.
3. **Context-dependent** — it is technically plausible, but the supplied files
   do not establish whether the comment is defective. This includes claims that
   require project conventions, external contracts, callers, tests, or related
   definitions not present in the evaluation material.
4. **Invalid** — it is contradicted by supplied source, duplicates another
   finding, asks for information the source already provides, or proposes only
   immaterial/optional polish.
5. **Non-comment target** — it targets executable code, a string, a Python
   docstring, or other non-comment text.

Score fields:

- `matched_expected_findings`: distinct reference findings correctly identified
  by the report.
- `valid_additional_findings`: material findings supported by supplied source
  that do not match a reference finding.
- `context_dependent_findings`: plausible findings that cannot be confirmed or
  rejected from supplied context.
- `invalid_findings`: reported comment findings that are wrong, duplicate,
  already answered, or immaterial optional polish.
- `finding_classifications`: one classification object for every reported
  finding, in exact `review.json` order. Its length must equal the number of
  reported findings. Each object must be `{"predicted":"<class>"}` where class
  is `matched_expected`, `valid_additional`, `context_dependent`, `invalid`, or
  `non_comment`. The count of each class must equal its corresponding count
  field.
- `non_comment_targets`: findings aimed at code, strings, docstrings, or other
  non-comment text.
- `missed_expected_findings`: unmatched entries in `expected-findings.json`.
  This must equal the fixed reference count minus matched expected findings.
- `actionable_findings`: matched or valid-additional findings with a safe
  deletion/replacement or a necessary concrete clarification question.
  Context-dependent findings are not actionable yet.
- `estimated_recall`: `matched_expected_findings / (matched_expected_findings +
  missed_expected_findings)`. The denominator is the fixed reference set.
- `estimated_precision`: `(matched_expected_findings +
  valid_additional_findings) / (matched_expected_findings +
  valid_additional_findings + invalid_findings + non_comment_targets)`, or `1`
  when that denominator is zero. Context-dependent findings are excluded because
  the supplied material cannot adjudicate them.
- `replacement_safety`: 0–100 assessment of whether proposed rewrites/deletions
  preserve meaning established by supplied source. Use 100 when there are no
  unsafe replacements; do not penalize a run merely because it reports no
  rewrites. A context-dependent replacement cannot receive full safety credit
  when its new claims require unavailable context.
- `overall_quality`: 0–100 holistic score emphasizing locally verified
  correctness, target safety, and useful precision, then recall and
  actionability. A bounded number of clearly labeled context-dependent concerns
  is acceptable; unsupported volume still reduces quality.

Return only the requested JSON score object. Do not include explanations or
additional keys.
