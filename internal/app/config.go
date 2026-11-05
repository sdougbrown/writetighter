package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/sdougbrown/writetighter/internal/config"
	"github.com/sdougbrown/writetighter/internal/llm"
	"github.com/sdougbrown/writetighter/internal/setup"
)

// configProbeTimeout bounds the targeted-update model discovery and response
// mode probes.
const configProbeTimeout = 45 * time.Second

// RunConfigUpdate applies a targeted --model/--code-model change to the
// existing user config, preflighting the model against the endpoint, and
// prints the sanitized result. It requires an existing valid config.
func (a *App) RunConfigUpdate(ctx context.Context, stdout, stderr io.Writer, model, codeModel string) error {
	existing, err := config.LoadUserConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("no existing config; run 'writetighter config --wizard' first")
		}
		return fmt.Errorf("failed to load config: %w", err)
	}
	if existing.LLM.BaseURL == "" {
		return fmt.Errorf("config is missing base_url; run 'writetighter config --wizard' first")
	}

	if model != "" {
		apiKey := resolveAPIKey(existing)
		ctx, cancel := context.WithTimeout(ctx, configProbeTimeout)
		defer cancel()

		models, discErr := llm.ListModels(existing.LLM.BaseURL, apiKey, configProbeTimeout)
		if discErr != nil || len(models) == 0 {
			if discErr != nil {
				fmt.Fprintf(stderr, "Warning: model discovery unavailable: %v; proceeding with explicit model %q.\n", discErr, model)
			} else {
				fmt.Fprintf(stderr, "Warning: model discovery returned no models; proceeding with explicit model %q.\n", model)
			}
		} else {
			found := false
			for i := range models {
				if models[i].ID == model {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("model %q was not reported by the endpoint", model)
			}
		}

		existing.LLM.Model = model

		client := &http.Client{Timeout: configProbeTimeout}
		newMode, probeErr := setup.ProbeResponseMode(ctx, client, existing.LLM.BaseURL, model, apiKey)
		if probeErr != nil {
			fmt.Fprintf(stderr, "Warning: response mode preflight failed for %q: %v\n", model, probeErr)
		} else {
			existing.LLM.ResponseMode = newMode
			fmt.Fprintf(stderr, "Response mode refreshed to %q for model %q.\n", newMode, model)
		}
	}

	if codeModel != "" {
		existing.LLM.CodeModel = codeModel
	}

	path, err := config.WriteUserConfig(existing)
	if err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	tomlStr, err := existing.SanitizedTOML()
	if err != nil {
		return fmt.Errorf("failed to render config: %w", err)
	}
	fmt.Fprint(stdout, tomlStr)
	fmt.Fprintf(stderr, "\nWrote %s\n", path)
	return nil
}

// RunConfigShow prints the sanitized user config when a model is configured,
// and reports whether it did. A missing or unconfigured config reports false
// so the caller can offer the wizard.
func (a *App) RunConfigShow(stdout, stderr io.Writer) (bool, error) {
	existing, err := config.LoadUserConfig()
	if err != nil || existing == nil || existing.LLM.Model == "" {
		return false, nil
	}
	tomlStr, err := existing.SanitizedTOML()
	if err != nil {
		return false, fmt.Errorf("failed to render config: %w", err)
	}
	fmt.Fprint(stdout, tomlStr)
	if existing.LLM.APIKey != "" {
		fmt.Fprintln(stderr, "\n(API key is stored in config.toml but redacted here.)")
	}
	fmt.Fprintln(stderr, "To reconfigure, run: writetighter config --wizard")
	return true, nil
}

// resolveAPIKeyParts resolves an API key from a direct value with an
// environment-variable fallback. It is the single owner of that rule for
// the app layer (config preflight and revise/rewrite endpoint queries).
func resolveAPIKeyParts(apiKey, apiKeyEnv string) string {
	if apiKey != "" {
		return apiKey
	}
	if apiKeyEnv != "" {
		return os.Getenv(apiKeyEnv)
	}
	return ""
}

func resolveAPIKey(cfg *config.UserConfig) string {
	return resolveAPIKeyParts(cfg.LLM.APIKey, cfg.LLM.APIKeyEnv)
}
