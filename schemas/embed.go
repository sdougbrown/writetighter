// Package schemas holds public JSON Schema definitions embedded in the
// WriteTighter binary. The files in this directory are the source of truth
// for the schemas sent to OpenAI-compatible endpoints. They are also
// discoverable at https://github.com/sdougbrown/writetighter/tree/main/schemas.
package schemas

import _ "embed"

// ReviseResponseSchemaV1 is the JSON Schema for the structured revision
// response sent to models when response_mode = "json_schema". The
// {{PRINCIPLE_IDS}} placeholder in the principle_ids enum is replaced at
// runtime. It becomes the JSON array of active principle IDs from the
// loaded guidance profile.
//
//go:embed revise-response-v1.schema.json
var ReviseResponseSchemaV1 string

// CodeCommentResponseSchemaV1 is the strict, catalog-ID response schema for
// code-aware revision. Its principle placeholder is populated at runtime.
//
//go:embed code-comment-response-v1.schema.json
var CodeCommentResponseSchemaV1 string
