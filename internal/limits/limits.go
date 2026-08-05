package limits

// MaxFileBytes is the maximum size for a single file (5 MiB).
const MaxFileBytes = 5 * 1024 * 1024

// MaxAggregateBytes is the maximum total input size across all selected files (25 MiB).
const MaxAggregateBytes = 25 * 1024 * 1024
