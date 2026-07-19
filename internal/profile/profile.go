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
}
