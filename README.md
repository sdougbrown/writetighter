# WriteTighter

WriteTighter is a local technical-writing revision harness for Markdown. Its primary command, `revise`, asks a configured OpenAI-compatible model for structured rewrites or clarification requests. Its deterministic companion, `lint`, reports narrow, auditable profile findings.

**Status:** Private development. The embedded default is `software-docs-en@0.2.0`. This repository is not currently cleared for public redistribution.

## What WriteTighter does

### Contextual revision

`revise` independently analyzes every selected document; it does not require a prior `lint` run or any lint findings. It returns source ranges, reasons, confidence values, and either a suggested replacement or a clarification question.

`revise` only returns advice. A human reviewer or the calling agent decides what to change; WriteTighter never edits the source file.

The revision rubric favors concise, direct instructions; consistent reviewed terminology; short, single-topic paragraphs; and explicit relationships between subjects, actions, transformations, and effects. Passage-matching profile policy and project glossary definitions provide terminology context. Corpus-only observed vocabulary does not become policy.

### Deterministic lint

`lint` provides measurements and exact policy checks that do not require a model:

- **Sentence length:** Applies profile thresholds for PRs, procedures, and descriptions.
- **Dense paragraphs:** Reports candidate findings for blocks that exceed configured sentence or word counts.
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
curl -fsSL https://raw.githubusercontent.com/sdougbrown/writetighter/main/install.sh | bash
```

This downloads the latest release binary for your platform to `~/.local/bin`.
Set `BINDIR` to choose a different location:

```sh
curl -fsSL https://raw.githubusercontent.com/sdougbrown/writetighter/main/install.sh | BINDIR=/usr/local/bin bash
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

`config` creates a working user configuration in the platform-specific XDG path (normally `~/.config/writetighter/config.toml`). It accepts a complete API URL, a `host:port`, or a localhost port.

The workflow queries the OpenAI-compatible model list and lets you select a model. It sends a small chat request to verify that the model returns a JSON object, selecting `json_object` mode when supported and `prompt_json` otherwise. After the preflight succeeds, it atomically writes the config with `0600` permissions.

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

`revise` runs independently of `lint` findings because concise text can still omit the subject, transformation, or effect while satisfying deterministic thresholds.

**Output:** A structured JSON response containing revision suggestions, each with:
- top-level `sources`: every document selected, including documents with no suggestions
- top-level `analysis`: input bytes, analyzed bytes, chunk count, and complete-coverage status for each source
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

A project term that does not conflict with the profile does not need `override`.
A term that conflicts with a discouraged profile entry must set `override = true`
and provide a non-empty reason.
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
max_requests = 32
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

`revise` analyzes every selected document regardless of static findings. It reads model settings from the user config file; it has no model-related command-line flags.

The command splits large documents at Markdown block boundaries and sends each chunk sequentially to the configured endpoint. Returned revisions retain original-document ranges. The response reports whether every byte was analyzed.

The default `max_requests = 32` prevents an unexpectedly large input from producing hundreds of model calls. Increase it deliberately in user config when a document requires more chunks. `revise` returns structured `rewrite` or `clarification` revisions and never modifies target files.

**Example:**
```sh
./writetighter revise docs/

./writetighter revise --text "Which transformation changes these values?"
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
