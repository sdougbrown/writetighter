# prose-extract

Extracts clean prose passages from markdown/doc sources for building the
WriteTighter prose-revision training corpus (Adapter B). Reuses the
`internal/document` segmenter so code blocks, inline code, HTML, front
matter, and link destinations are protected and excluded from the prose.

Inline code is replaced with the placeholder `<code>` so sentences stay
grammatical.

Usage:

    prose-extract <file|dir>...

Output: JSON lines, one per passage:

    {"source":"...","file":"...","text":"...","words":N}

Experimental fixture tooling for the post-training spike -- not a WriteTighter
product feature.
