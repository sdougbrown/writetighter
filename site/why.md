# Why

## The Evolution of Slop

We've moved past the era of "polite" AI slop. The old problem was simple verbosity: the hedges, the throat-clearing, and the endless "it's important to note" preambles. That was just noise.

Today, we face **Combat Slop**.

Combat Slop is a new, more aggressive breed of linguistic density. It is produced by models that have been RL-tuned for extreme precision but have no concept of human cognitive load. It manifests as hyper-dense, structurally complex prose—sentences that attempt to pack a Jira ticket, a technical implementation, and three nested logical constraints into a single, unparseable string.

Every sentence is a logic puzzle. Every paragraph is a manual decompression task.

## The Cost

This density is a tax on both humans and machines. 

For humans, it’s a cognitive tax. It forces us to perform "mental parsing" just to find the signal. For models—whether they are reviewing a PR or summarizing a fix—it is a literal token tax. We are wasting computational energy navigating linguistic scaffolding just to reach the underlying fact.

`writetighter` is built to strip away the scaffolding and return the signal.

## Limitations

WriteTighter doesn't (currently) grade prose, certify compliance with a standard, or detect whether text was written by a human or a model. It has no opinion on tone, only on whether a sentence is clear and easy to understand. If a sentence is long, dense, hedged, or vague *and* every word in it is load-bearing, `lint` and `revise` say so.
