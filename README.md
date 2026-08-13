# WriteTighter

WriteTighter is a local technical-writing revision harness for Markdown. Its primary command, `revise`, asks a configured OpenAI-compatible model for structured rewrites or clarification requests. Its deterministic companion, `lint`, reports narrow, auditable profile findings.

**Status:** Private development. The embedded default is `software-docs-en@0.5.0`. This repository is not currently cleared for public redistribution.

## What WriteTighter does

### Contextual revision

`revise` independently analyzes every selected document; it does not require a prior `lint` run or any lint findings. It returns source ranges, reasons, confidence values, and either a suggested replacement or a clarification question.

`revise` only returns advice. A human reviewer or the calling agent decides what to change; WriteTighter never edits the source file.

The revision rubric favors direct instructions, consistent reviewed terminology, single-topic paragraphs, literal technical mechanisms, and explicit relationships between subjects, actions, transformations, and effects. It may reorder established details into cause, implication, action, and effect. It uses enough detail to explain the mechanism instead of optimizing for the fewest words.

Passage-matching profile policy and project glossary definitions provide terminology context. Corpus-only observed vocabulary does not become policy.

### Deterministic lint

`lint` provides measurements and exact policy checks that do not require a model:

- **Sentence length:** Applies profile thresholds for PRs, procedures, and descriptions.
- **Dense paragraphs:** Reports candidate findings for blocks that exceed configured sentence or word counts.
- **Noun stacks:** Flags long runs of consecutive content words that may need unpacking.
- **Gerund openers:** Flags sentences that open with a gerund phrase.
- **Contractions:** Flags common English contractions (n't, 'll, 're, 've, 'd, it's) in prose.
- **Banned modals:** Flags STE-unapproved modal verbs (should, would, may, might, could).
- **Latin abbreviations:** Flags e.g., i.e., and etc. in prose.
- **Term policy:** Supports discouraged terms, canonical case, unknown terms, and project term bases when the selected profile enables those rules.

## Safety boundaries

- **No automatic application:** `revise` suggests changes but never writes them.
- **No guessing:** The rubric requests clarification when a safe rewrite would require assumptions.
- **No invented details:** The rubric prohibits adding facts, actors, identifiers, examples, or implementation details.
- **Protected technical content:** Rewrites that lose commands, code, identifiers, paths, URLs, numbers, versions, product names, or defined project terms are discarded.
- **No authorship detection:** WriteTighter improves prose; it does not classify text as human- or AI-written.
- **No certification:** Findings and suggestions do not establish compliance with a standard.
- **No universal vocabulary gate:** Reviewed terminology can guide a passage, but corpus frequency and incomplete dictionaries do not define every acceptable word.

## Installation

### Quick install (curl)

```sh
curl -fsSL https://writetighter.douggo.com/install.sh | bash
```

This downloads the latest release binary for your platform to `~/.local/bin`.
Set `BINDIR` to choose a different location:

```sh
curl -fsSL https://writetighter.douggo.com/install.sh | BINDIR=/usr/local/bin bash
```

Pre-release versions are marked as pre-releases on GitHub and are excluded
from the `latest` tag the installer resolves.

### Go install

If you have Go 1.22+ installed:

```sh
go install github.com/sdougbrown/writetighter/cmd/writetighter@latest
```

### Build from source

```sh
git clone https://github.com/sdougbrown/writetighter.git
cd writetighter
go build -o writetighter ./cmd/writetighter
```

### Current default policy

The embedded `software-docs-en@0.5.0` profile uses these limits:

| Document kind | Sentence limit |
|---|---:|
| `pr` | 20 words |
| `procedure` | 20 words |
| `description` | 25 words |
| `code-comment` | 25 words (inherits `description`) |
| `reference` | 25 words (inherits `description`) |
| `decision` | 25 words (inherits `description`) |
| `incident` | 25 words (inherits `description`) |
| `agent-instruction` | 25 words (inherits `description`) |
| `status-update` | 25 words (inherits `description`) |

It also emits an informational candidate when a paragraph has more than three
sentences or more than 80 words. The profile carries a reviewed dictionary
with discouraged-term policies for common filler and AI-slop vocabulary,
enabled as a non-enforcing `candidate`/`info` rule.

Two non-enforcing `candidate`/`info` rules keep jargon checks precise. `lint` reports a short all-caps abbreviation (`EI`, `NPE`) only when
it first appears with no parenthetical expansion anywhere in the document
(common technical abbreviations such as `API`/`ID`/`URL` are excluded, and an
expansion in either order — `EI (Ending Inventory)` or `Ending Inventory (EI)`
— suppresses the finding). Noun-stack detection stops at clause boundaries
(em/en dash, semicolon, colon), rejects windows that read as clauses or end in
a participle (`unit conversions use`, `barcode registered`), and in
`code-comment`/`pr` prose ignores identifier- or acronym-heavy stacks
(``primaryCategoryId``, ``Fabric StateWrapper``).

## Commands

### `writetighter config` (Interactive Setup)

`config` creates a working user configuration in the platform-specific XDG path (normally `~/.config/writetighter/config.toml`). It accepts a complete API URL, a `host:port`, or a localhost port.

The workflow queries the OpenAI-compatible model list and lets you select a model. It sends a small chat request to verify that the model supports structured JSON output, selecting `json_schema` mode when supported, then falling back to `json_object`, then `prompt_json`. After the preflight succeeds, it atomically writes the config with `0600` permissions.

```sh
./writetighter config
```

The workflow offers three authentication choices: no key, a key stored in the private user config, or an environment-variable reference. Keys are entered without terminal echo and are never accepted as command-line arguments. An interactive `revise` invocation automatically starts this workflow when model configuration is missing or invalid; non-interactive invocations fail with a `writetighter config` hint instead of waiting for input.

### `writetighter lint` (Deterministic)

`lint` runs deterministic profile rules. It never invokes a model. With `--kind code-comment`, explicit source files in the same supported languages as code-aware `revise` are lexed first, so rules inspect only cataloged comments and findings retain their original file ranges. `--stdin`, `--text`, and unsupported extensions retain prose-style linting.

```sh
./writetighter lint README.md --kind description --format json
```

### `writetighter prompt` (Reusable Revision Guidance)

`prompt` prints the same core and kind-specific directions used by `revise`. It does not read input, load model configuration, resolve a profile, or invoke a model. An agent can pass the human output directly to a subagent or consume the versioned JSON structure.

```sh
./writetighter prompt --kind code-comment
./writetighter prompt --kind decision --format json
```

Supported kinds provide these revision lenses:

- `description`: mechanical understanding, causal explanation, and purposeful restatement
- `procedure`: prerequisites, ordered actions, expected results, and operational exceptions
- `pr`: review-relevant purpose, dependencies, behavior, and decisions
- `code-comment`: non-obvious constraints, invariants, rationale, contracts, and effects
- `reference`: accurate lookup, exact definitions, defaults, constraints, and failure conditions
- `decision`: context, drivers, alternatives, tradeoffs, the selected approach, and consequences
- `incident`: observed facts, impact, chronology, causal confidence, and corrective actions
- `agent-instruction`: executable objectives, inputs, tools, decision points, outputs, verification, and failure behavior
- `status-update`: observable progress, evidence, blockers or hypotheses, next actions, and reporting points

The specialized lenses ask for clarification instead of inferring missing code behavior, defaults, decision provenance, incident causes, agent capabilities, or operational diagnoses.

`revise --kind agent-instruction` remains an exact-range prose reviewer. Missing steps and contradictions across distant sections may not have one contiguous evidence range.

For holistic skill or prompt review, pass `writetighter prompt --kind agent-instruction` and the full instruction to a capable reasoning agent. The wrapping agent still verifies and applies any proposed change.

For Claude Code and Codex, a packaged plugin wraps this workflow as an installable `tighten` skill — see the [Claude Code plugin guide](https://writetighter.douggo.com/claude-plugin) and the [Codex plugin guide](https://writetighter.douggo.com/codex-plugin).

### `writetighter revise` (Opt-In Contextual Revision)

`revise` runs contextual revision with the configured model. It reads LLM configuration from your user config (`~/.config/writetighter/config.toml [llm]` section) and never modifies target files.

`revise` runs independently of `lint` findings because concise text can still omit the subject, transformation, or effect while satisfying deterministic thresholds.

For `revise --kind code-comment`, explicit Go, TypeScript/JavaScript (`.ts`, `.tsx`, `.js`, `.jsx`, `.mjs`, `.cjs`), Rust, Python, and `.pyi` files, plus shell scripts (`.sh`, `.bash`, `.zsh`, `.ksh`), use lexer-owned comment IDs. The complete file is read-only model context; only cataloged comments can become findings, and source text and ranges in the report always come from that catalog. The path sends one whole-file request and rejects unsafe, low-confidence suggestions. `--stdin`, `--text`, and unsupported extensions retain the prose-style code-comment path. Code-aware file review currently rejects `--reference` rather than silently omitting that context.

**Output:** A structured JSON response containing revision suggestions, each with:
- top-level `sources`: every document selected, including documents with no suggestions
- top-level `analysis`: input bytes, analyzed bytes, chunk count, model-request count, and complete-coverage status for each source
- `kind`: `"rewrite"` or `"clarification"`
- `source_path`: File path
- `source_text`: Exact source evidence copied from the selected range
- `range`: Source byte range and line/column positions
- `principle_ids`: Stable revision principles that explain the suggestion
- `reason`: Explanation
- `replacement` (rewrite only): Suggested replacement text
- `question` (clarification only): Concrete question instead of fabricated rewrite
- `confidence`: Float between 0 and 1
- `discarded_rewrites`: Count of model rewrites suppressed because they lost protected technical content
- `discarded_findings`: Count of code-comment findings rejected for unknown targets, malformed payloads, low confidence, or unsafe replacements
- `errors`: Per-document model or response failures; their presence also produces exit code 3

**Requirements:**
- Model configuration must be present in the user config (model and base_url are required); `writetighter config` creates and preflights it
- Local endpoints may omit authentication
- Authenticated endpoints may use either `api_key` in the private config or `api_key_env`, but not both
- API keys are never accepted as command-line arguments

**Examples:**
```sh
./writetighter revise README.md --kind description

printf '%s\n' "Restart the service after changing the file." |
  ./writetighter revise --stdin --kind procedure

./writetighter revise --text "Which transformation changes these values?"

./writetighter revise internal/app/app.go --kind code-comment

# --text remains the prose-style code-comment fallback.
./writetighter revise --text "Wait for the marker before reading configuration." \
  --kind code-comment
```

### Reference context

`revise` can include optional reference files and directories that provide
broader factual context for revision decisions. Reference material is read-only:
it is never linted, returned as a rewrite range, or included in the output.

```sh
./writetighter revise docs/ --reference style-guide.md
./writetighter revise README.md --reference docs/ --reference glossary.json
```

Reference paths are invocation-specific and must exist at the time of the call.
Directories are recursively scanned; only recognized text and code file types
are included. Files starting with `.env`, secret-key files, well-known
credential files (e.g. `kubeconfig`, `credentials.json`, `service-account*.json`,`
application_default_credentials.json`, `.npmrc`/`.netrc`/`.pypirc`/`.dockercfg`, terraform
`*.tfvars`), symlinks, binary data, and hidden directories are excluded. Files
whose content carries strong credential markers (PEM private-key blocks,
`private_key`/`client_secret`/`refresh_token`/`access_token` fields) are also
refused: an explicitly-passed such file errors out, and one found inside a
reference directory is skipped with a warning. Files or directories whose
canonical path matches a source file being revised are automatically excluded.

Reference content is sent to the configured endpoint. The response reports
reference metadata (files, bytes, completeness) in the `reference_context` field
without exposing the reference content itself. Files skipped during collection
(for example, symlinks encountered inside a reference directory) are listed
under `reference_context.warnings` so nothing is dropped silently.

**Requirements:**
- Reference revision auto-detects the model context window from the
  `/v1/models` endpoint. Use `--context-tokens` to override it if the endpoint
  does not report context metadata.
- If the context window is too small for the combined system prompt,
  references, and editable source, `revise` reports an error before making any
  model call.

---

Configure the model before using `revise --stdin`. Standard input carries the document, so that invocation cannot also run the interactive configuration workflow. Use `--text` for short, non-sensitive text when you want stdin to remain available for interactive setup.

## Basic Linting

You can lint text via `stdin` or by providing paths to files and directories.

**Lint via stdin:**

```sh
echo "This change replaces the global mutex with per-repository locks so unrelated repository updates no longer block one another during mirror fetch operations." |
  ./writetighter lint --stdin --kind pr
```

The human report includes the static finding:

```text
status: linted
warning CORE.SENTENCE_LENGTH Split this sentence into independently verifiable claims.
```

**Lint a direct string:**

```sh
./writetighter lint --text "Restart the service after changing the file."
```

`--text`, `--stdin`, and file paths are mutually exclusive. Command-line text may appear in shell history or process listings, so prefer `--stdin` for large or sensitive content.

**Lint one file:**

```sh
./writetighter lint README.md --kind description --format human
```

**Lint a directory of Markdown files:**
```sh
./writetighter lint docs/ --kind description --format json
```

### Output Formats and Exit Codes

`--format` defaults to `human`. WriteTighter supports three output formats:
- `human`: Readable text for developers.
- `json`: Structured data for tool integration.
- `agent`: Compact, line-oriented output for coding agents.

**Exit Codes:**
- `0`: `lint` completed without reaching the failure threshold, or `revise` completed successfully.
- `1`: `lint` findings reached the `--fail-on` threshold.
- `2`: Usage, configuration, profile, or input failure.
- `3`: A required `revise` model call or response failed.

You can control failure behavior with `--fail-on`:
- `--fail-on none` (default): Always exit `0`.
- `--fail-on warning`: Exit `1` if any `warning` or `error` findings exist.
- `--fail-on error`: Exit `1` if any `error` findings exist.

### Profiles

WriteTighter uses profiles to define its logic. The embedded default is `software-docs-en@0.5.0`.

**Manage profiles:**
```sh
# List installed profiles (including the embedded one)
./writetighter profile list

# Verify a specific profile or bundle path
./writetighter profile verify software-docs-en@0.5.0

# Install a new profile bundle from a local path
./writetighter profile install ./path/to/bundle
```

Pin profiles as `ID@VERSION` to keep results consistent across environments.

### Configuration and Term-bases

WriteTighter searches upward from the current working directory for
`.writetighter.toml`. Pass `--config PATH` to bypass discovery.

**Project configuration example:**
```toml
[profile]
id = "software-docs-en"
version = "0.5.0"

[[terms]]
term = "hydrate"
parts_of_speech = ["verb"]
definition = "Restore serialized application state."

[[terms]]
term = "utilize"
parts_of_speech = ["verb"]
definition = "The name of the project's public API operation."
override = true
reason = "The published API uses this exact operation name."
```

A project term that does not conflict with the profile does not need `override`.
A term that conflicts with a discouraged profile entry must set `override = true`
and provide a non-empty reason.
Project configuration cannot contain LLM settings or credentials.

## Local Revision Model

Model configuration is always read from the user config file. Run `writetighter config` for guided endpoint discovery, model selection, and preflight. Never pass API keys on the command line.

Switch models or set a code-comment model without editing TOML:

```sh
writetighter config --model gemma4
writetighter config --code-model qwen-coder
```

Put machine-specific LLM settings in `~/.config/writetighter/config.toml` (or the matching XDG config path):

```toml
[llm]
provider = "openai-compatible"
base_url = "http://sparky:4000/v1"
model = "gemma4"
response_mode = "json_schema"
max_requests = 32
# Optional: use a different model for code-aware comment revision
# code_model = "qwen-coder"
```

`response_mode` controls how the model is asked to produce structured output. `json_schema` sends a full JSON Schema (see `schemas/revise-response-v1.schema.json`). It is preferred for smaller models that benefit from grammar-constrained generation. `json_object` sends a lighter `{"type":"json_object"}` hint. `prompt_json` relies on prompt instructions only and is the universal fallback. The setup wizard discovers the best supported mode automatically.

For an authenticated endpoint, choose one credential source. A lower-risk local PAT can be stored in the `0600` user config:

```toml
api_key = "pat-value"
```

For credentials you do not want WriteTighter to persist, configure an environment-variable name instead:

```toml
api_key_env = "WRITETIGHTER_API_KEY"
```

### Revision analysis

`revise` analyzes every selected document regardless of static findings. It reads model settings from the user config file. Use `--model` to override the configured model for a single run, or `--code-model` to use a different model for code-aware comment revision. Use `--context-tokens` and `--output-tokens` to override the auto-detected context window and max output tokens.

The command splits large Markdown documents at block boundaries and sends each chunk sequentially to the configured endpoint. Returned Markdown and text revisions retain original-document ranges. The response reports whether every byte was analyzed.

The default `max_requests = 32` prevents an unexpectedly large input from producing hundreds of model calls. The limit counts initial chunk requests and runtime fallbacks. Increase it deliberately in user config when a document requires more chunks.

When `json_object` or `json_schema` assistant content violates the revision contract, `revise` retries that chunk once with `prompt_json` if the request budget can still cover every remaining chunk. A model-returned `error` object is reported as a model error rather than an unknown JSON field. `revise` returns structured `rewrite` or `clarification` revisions and never modifies target files.

### HTML input

Direct `.html` and `.htm` files, including those discovered in directories, are supported. WriteTighter extracts visible text while excluding comments and the `head`, `script`, `style`, `template`, and `noscript` subtrees. This is not browser rendering: CSS and JavaScript visibility are not evaluated.

HTML revisions declare `source_format: "html"` and `range_basis: "visible_text"`. Their `source_spans` identify the related original HTML byte ranges; visible-text offsets are never presented as HTML offsets. Suggestions remain advisory, and WriteTighter never modifies the HTML file.

**Example:**
```sh
./writetighter revise docs/

./writetighter revise --text "Which transformation changes these values?"
```

### Security: Secret Handling
Never pass API keys as command-line arguments. The setup workflow can save a key in the user-only `config.toml` (`0600`) or store only an environment-variable name. Use environment-variable mode for credentials whose persistence policy forbids local config storage.

### Context capacity and budgeting

`revise` auto-detects the model context window from the `/v1/models` endpoint
when needed (reference revision or code-aware comment revision). Use
`--context-tokens` to override it, or `--output-tokens` to override the max
output tokens sent to the API. When the context window is known, `revise`
reserves output tokens, accounts for the system prompt, response format,
reference content, and a safety margin before allocating tokens to editable
source text. Token estimates use a 4-bytes-per-token heuristic. A hard byte
ceiling (`MaxInputChars`) provides compatibility with endpoints that enforce
byte limits.

If the budget cannot accommodate every source chunk, `revise` reports an
actionable error before making model calls. It never silently truncates source
or reference content.

For plain prose revision without references, `revise` falls back to legacy
byte-budget chunking and does not send `max_tokens` to the API, preserving
backward-compatible behavior.

## Practical Workflows

**Checking PR descriptions:**
```sh
./writetighter lint pr_description.md --kind pr
```

**Checking recent Markdown plans:**
```sh
./writetighter lint plans/*.md --kind description --format json
```

## Known Limitations (Pre-release)

As this is a private development build, please be aware of the following:
- **Profile Rights:** Public redistribution of profiles is currently restricted.
- **Heuristic Splitting:** Sentence and paragraph splitting uses heuristics and may occasionally miscalculate spans.
- **Narrow Policy:** The current profile has only two reviewed lexical-policy entries and does not yet enable its deterministic term rules.
- **Precision Gate:** The automated precision/recall for rule enforcement is still being calibrated.
- **No Auto-apply:** `lint` reports deterministic findings and `revise` returns suggested replacements or questions, but neither command modifies files.

## Documentation

- [Profile model and term-base boundaries](docs/decisions/0002-profile-model.md)
- [Product naming decision](docs/decisions/0001-product-name.md)
- [Data provenance and rights](docs/data-provenance.md)
- [Agent workflow](docs/agent-workflow.md)
- [Claude Code plugin](https://writetighter.douggo.com/claude-plugin) and [Codex plugin](https://writetighter.douggo.com/codex-plugin)
- Example configs in [`examples/`](examples/)
