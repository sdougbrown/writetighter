# Why Tighter Prose

## <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round" class="wt-icon" aria-hidden="true"><path d="M19 3c-6 0-11 4-13 10-1 3 .5 6 3 7 6-2 10-7 10-13 0-1.5 0-3-.5-4Z"/><path d="M9 20 4 21"/><path d="M12 12c-1.5 1-3 1.8-4.5 2.2"/><path d="M14.5 9c-1.5 1-3.2 2-5 2.6"/><path d="M16.5 6.3c-1.3.8-2.8 1.6-4.4 2.2"/></svg> The wordslop problem

Heavily RL-tuned models write a specific dialect: hedges stacked three deep,
throat-clearing before the point, verbs turned into nouns.
Every fix gets described as a "comprehensive, robust" solution, whether or
not it touched more than one line. None of it is wrong, exactly. All of it is
padding — the model reaching for a safe, agreeable register instead of a
precise one.

That dialect is expensive twice over. A human reader spends longer finding
the actual claim buried in the hedges. When the reader is another model —
reviewing the PR, summarizing the doc, deciding next steps — every padded
sentence costs tokens to hold. It pays those tokens just to discard the
padding and keep the fact underneath.

## Prose is a budget

An agent reading a bloated README or a hedge-stacked PR description isn't
just reading slower. It's spending context window on words that carry no
decision-relevant information, in a window it will need later for the actual
task. Tight prose isn't a style preference in that setting. It's the
difference between a docs page that costs 200 tokens to absorb and one that
costs 600 for the same three facts.

## What WriteTighter actually checks

Two passes, one deterministic, one contextual — see the
[CLI reference](/cli) for the exact commands:

- **`lint`** catches what's measurable without a model: sentence length past
  a kind-specific limit, paragraphs dense enough to bury their own topic.
- **`revise`** catches what needs judgment: hedges without real uncertainty,
  verbs turned into nouns, words pointing at something never said,
  scope-inflating framing around a plain fact.

Neither pass edits anything. Both report findings a human or an agent
verifies and applies — see [Before / After](/before-after) for what that
looks like in practice.

## What it doesn't do

It doesn't grade prose, certify compliance with a standard, or detect
whether text was written by a human or a model. It has no opinion on tone,
only on whether a sentence is carrying its own weight. If a sentence is
long, dense, hedged, or vague *and* every word in it is load-bearing,
`lint` and `revise` say so — a `candidate` finding, not a required fix.
