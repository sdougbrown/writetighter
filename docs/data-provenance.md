# Data provenance and rights

Last updated: 2026-07-19

This file records every data source that informed the `software-docs-en` profile dictionary. Each row describes the exact revision/release, source URL, license, which fields were copied or derived, any attribution requirement, and the current redistribution decision.

## Sources

| # | Source | Revision/Release | URL | License | Copied/Derived | Attribution | Redistribution |
|---|---|---|---|---|---|---|---|
| 1 | Go documentation corpus | Go 1.22 release | https://github.com/golang/go | BSD-3-Clause | Derived word frequency counts from processed sentences | Recommended | Not reviewed; all entries are `observed` only |
| 2 | Kubernetes documentation corpus | v1.30 | https://github.com/kubernetes/website | CC-BY-4.0 | Derived word frequency counts from processed sentences | Required as CC-BY-4.0 upstream | Not reviewed; all entries are `observed` only |
| 3 | Ruby on Rails documentation corpus | v7.1 | https://github.com/rails/rails | MIT | Derived word frequency counts from processed sentences | Not required | Not reviewed; all entries are `observed` only |
| 4 | Stripe documentation corpus | Current as of 2026-Q1 | https://docs.stripe.com | Proprietary/unknown | Derived word frequency counts from processed sentences | TBD | Not cleared. Exclude from any public release unless separately cleared. |
| 5 | OpenSTE | vX.Y | https://github.com/OpenSTE/open-ste | MIT | Used during comparison and construction of the dictionary | Recommended | Not reviewed; comparison-only use |

## Conversion gap

The research artifact at `corpus/controlled_vocab.json` contains 1,575 words with `word` and `freq` keys only. The published bundle converts these to `observed` entries with empty `parts_of_speech` and evidence references linking back to the source revision and frequency.

The following are not present in the research artifact and were not inferred:
- Reviewed policy status (`preferred`, `allowed`, `discouraged`)
- Reviewed parts of speech
- Alternatives
- Canonical case declarations
- Rule configuration

## Public release blockers

A public release of `software-docs-en@0.1.0` requires:
1. A completed source-by-source rights review for each corpus.
2. Exclusion or clearance of the Stripe corpus.
3. Selection of a public license for the derived dictionary.
4. Regeneration of the bundle with the public license and updated provenance.

## Non-goals

This file does not establish legal advice, grant permission, or certify license compliance. It records factual observations for the project maintainers.
