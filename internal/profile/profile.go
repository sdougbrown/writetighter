package profile

type ProfileID string

type Version string

type Resolution struct {
	ID       ProfileID
	Version  Version
	SHA256   string
	Source   string
	Manifest *Manifest
	Dict     *Dictionary
	Rules    *RulesConfig

	// Resolved dictionary bytes: read from the bundle (format 1/2) or
	// materialized from the content-addressed store (format 3). Install
	// writes these directly, so it can never resolve a different dictionary
	// than the one LoadBundle validated.
	dictBytes []byte
}
