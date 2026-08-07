package check

import (
	"github.com/sdougbrown/writetighter/internal/config"
	"github.com/sdougbrown/writetighter/internal/corpus"
	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/profile"
	"github.com/sdougbrown/writetighter/internal/report"
)

type RunContext struct {
	Document   *document.Document
	Profile    *profile.Resolution
	Terms      []config.TermEntry
	GitCompare *corpus.GitCompare
}

type Checker interface {
	ID() string
	Version() int
	Run(ctx *RunContext) ([]report.Finding, error)
}
