"""wt_evals — consumer-owned Longe driver for the WriteTighter code-comment eval.

Compares the production `writetighter revise --kind code-comment` subprocess
against a code-aware direct OpenAI-compatible prompt on a corpus of real source
files, normalizing both into a shared `review.json` artefact.
"""

__version__ = "0.1.0"

__all__ = ["__version__"]
