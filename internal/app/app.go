package app

import "errors"

type CheckParams struct {
	Paths           []string
	Stdin           bool
	Kind            string
	Profile         string
	ConfigPath      string
	Format          string
	LLM             bool
	RequireLLM      bool
	LLMBaseURL      string
	LLMModel        string
	LLMResponseMode string
	FailOn          string
}

type RunFunc func() error

type App struct{}

func New() *App { return &App{} }

func (a *App) RunCheck(_ CheckParams) error { return errors.New("not implemented") }

func (a *App) RunExplain(_ string) error { return errors.New("not implemented") }

func (a *App) RunExplainWithOptions(_ string, _ string, _ string) error {
	return errors.New("not implemented")
}

func (a *App) RunProfileInstall(_ string) error { return errors.New("not implemented") }

func (a *App) RunProfileList() error { return errors.New("not implemented") }

func (a *App) RunProfileVerify(_ string) error { return errors.New("not implemented") }
