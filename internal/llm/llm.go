package llm

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/profile"
	"github.com/sdougbrown/writetighter/internal/report"
)

func Advisor(ctx context.Context, config Config, doc *document.Document, res *profile.Resolution, findings []report.Finding) ([]report.Finding, error) {
	client, err := NewClient(config)
	if err != nil {
		return findings, err
	}
	prompt, passage := BuildPrompt(doc, res, findings)
	req := Request{Messages: []Message{{Role: "system", Content: prompt}, {Role: "user", Content: passage}}}
	if config.ResponseMode != "" && config.ResponseMode != "auto" {
		req.ResponseFormat = &ResponseFormat{Type: config.ResponseMode}
	}
	resp, err := client.Do(ctx, req)
	if err != nil {
		return findings, err
	}
	if len(resp.Choices) == 0 {
		return findings, errors.New("empty llm response")
	}
	validated, err := ValidateAdvisorResponse([]byte(resp.Choices[0].Message.Content), passage)
	if err != nil {
		return findings, err
	}
	return append(findings, validated...), nil
}

func Host(base string) string {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return base
	}
	return fmt.Sprintf("%s://%s", u.Scheme, u.Host)
}
