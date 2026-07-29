# WriteTighter

WriteTighter is a local, standalone Go CLI for technical prose. It provides deterministic profile-based lint findings and opt-in, structured revision suggestions from a configured local model.

**Status:** Private development. The embedded default is `software-docs-en@0.2.0`. This repository is not currently cleared for public redistribution.

## What WriteTighter does (and does not) do

WriteTighter separates deterministic policy from contextual revision. A versioned **profile** supplies dictionary and rule policy; the revision rubric supplies general technical-writing principles.

### It checks

- **Sentence length:** Applies profile thresholds for PRs, procedures, and descriptions.
- **Dense paragraphs:** Reports candidate findings for blocks that exceed configured sentence or word counts.
- **Term policy:** Supports discouraged terms, canonical case, unknown terms, and project term bases when the selected profile enables those rules.

### It does not do
- **Grammar checking:** It is not a general-purpose grammar checker.
- **Automatic rewriting:** It identifies issues and suggests improvements via LLM, but it will never modify your files.
- **Automatic compliance:** It provides findings based on rules; it does not "certify" your documentation.
- **Broad prose analysis:** It focuses on specific, rule-based metrics rather than subjective "quality."

## Local Setup

To use WriteTighter from this checkout, build the binary locally:

```sh
go build -o writetighter ./cmd/writetighter
```

Run `go install ./cmd/writetighter` instead if your Go bin directory is already on
`PATH`.

### Current default policy

The embedded `software-docs-en@0.2.0` profile uses these limits:

| Document kind | Sentence limit |
|---|---:|
| `pr` | 20 words |
| `procedure` | 20 words |
| `description` | 25 words |

It also emits an informational candidate when a paragraph has more than three
sentences or more than 80 words. The profile carries a small reviewed dictionary,
but version 0.2.0 does not enable deterministic term findings yet.

## Commands

### `writetighter config` (Interactive Setup)

`config` creates a working user configuration in the platform-specific XDG path (normally `~/.config/writetighter/config.toml`). It accepts a complete API URL, a `host:port`, or a localhost port. The workflow queries the OpenAI-compatible model list, lets you select a model, preflights structured output, and atomically writes the config with `0600` permissions.

```sh
./writetighter config
```

The workflow offers three authentication choices: no key, a key stored in the private user config, or an environment-variable reference. Keys are entered without terminal echo and are never accepted as command-line arguments. An interactive `revise` invocation automatically starts this workflow when model configuration is missing or invalid; non-interactive invocations fail with a `writetighter config` hint instead of waiting for input.

### `writetighter lint` (Deterministic)

`lint` runs deterministic profile rules. It never invokes a model.

```sh
./writetighter lint README.md --kind description --format json
```

### `writetighter revise` (Opt-In Contextual Revision)

`revise` runs contextual revision with the configured model. It reads LLM configuration from your user config (`~/.config/writetighter/config.toml [llm]` section) and never modifies target files.

`revise` runs independently of `lint` findings because semantic compression can occur in short text.

**Output:** A structured JSON response containing revision suggestions, each with:
- top-level `sources`: every document analyzed, including documents with no suggestions
- `kind`: `"rewrite"` or `"clarification"`
- `source_path`: File path
- `range`: Source byte range and line/column positions
- `principle_ids`: Stable revision principles that explain the suggestion
- `reason`: Explanation
- `replacement` (rewrite only): Suggested replacement text
- `question` (clarification only): Concrete question instead of fabricated rewrite
- `confidence`: Float between 0 and 1
- `discarded_rewrites`: Count of model rewrites suppressed because they lost protected technical content
- `errors`: Per-document model or response failures; their presence also produces exit code 3

**Requirements:**
- Model configuration must be present in the user config (model and base_url are required); `writetighter config` creates and preflights it
- Local endpoints may omit authentication
- Authenticated endpoints may use either `api_key` in the private config or `api_key_env`, but not both
- API keys are never accepted as command-line arguments

**Example:**
```sh
./writetighter revise README.md --kind description
```

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

WriteTighter uses profiles to define its logic. The embedded default is `software-docs-en@0.2.0`.

**Manage profiles:**
```sh
# List installed profiles (including the embedded one)
./writetighter profile list

# Verify a specific profile or bundle path
./writetighter profile verify software-docs-en@0.2.0

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
version = "0.2.0"

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

An addition does not need `override`. A project term that conflicts with a
discouraged profile term must set `override = true` and provide a non-empty reason.
Project configuration cannot contain LLM settings or credentials.

## Local Revision Model

Model configuration is always read from the user config file. Run `writetighter config` for guided endpoint discovery, model selection, and preflight. Never pass API keys on the command line.

Put machine-specific LLM settings in `~/.config/writetighter/config.toml` (or the matching XDG config path):

```toml
[llm]
provider = "openai-compatible"
base_url = "http://sparky:4000/v1"
model = "gemma4"
response_mode = "json_object"
```

For an authenticated endpoint, choose one credential source. A lower-risk local PAT can be stored in the `0600` user config:

```toml
api_key = "pat-value"
```

For credentials you do not want WriteTighter to persist, configure an environment-variable name instead:

```toml
api_key_env = "WRITETIGHTER_API_KEY"
```

### Revision analysis

Runs contextual revision on every selected document regardless of static findings. Always reads LLM config from the user config file (no `--llm` flags needed). It sends the document, up to the documented input limit, to that configured endpoint and returns a dedicated JSON structure with `revisions` (either `rewrite` or `clarification`). It never modifies target files.

**Example:**
```sh
./writetighter revise docs/
```

### Security: Secret Handling
Never pass API keys as command-line arguments. The setup workflow can save a key in the user-only `config.toml` (`0600`) or store only an environment-variable name. Use environment-variable mode for credentials whose persistence policy forbids local config storage.

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
