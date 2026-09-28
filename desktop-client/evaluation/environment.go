package evaluation

// This file records the local Ollama and runtime identity used for a reproducible run.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"
)

type ollamaVersionResponse struct {
	Version string `json:"version"`
}

type ollamaTagsResponse struct {
	Models []struct {
		Name   string `json:"name"`
		Model  string `json:"model"`
		Digest string `json:"digest"`
	} `json:"models"`
}

// InspectEnvironment verifies Ollama reachability and records the selected model digest when available.
func InspectEnvironment(ctx context.Context, baseURL, model string) (Environment, error) {
	environment := Environment{
		OperatingSystem: runtime.GOOS, Architecture: runtime.GOARCH,
		LogicalCPUs: runtime.NumCPU(), GoVersion: runtime.Version(), ConfiguredModel: model,
	}
	root := strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/api")
	client := &http.Client{Timeout: 15 * time.Second}
	if err := getJSON(ctx, client, root+"/api/version", &ollamaVersionResponseTarget{environment: &environment}); err != nil {
		return Environment{}, fmt.Errorf("inspect Ollama version: %w", err)
	}
	var tags ollamaTagsResponse
	if err := getJSON(ctx, client, root+"/api/tags", &tags); err != nil {
		return Environment{}, fmt.Errorf("inspect Ollama models: %w", err)
	}
	for _, candidate := range tags.Models {
		if modelMatches(candidate.Name, model) || modelMatches(candidate.Model, model) {
			environment.Model = candidate.Name
			environment.ModelDigest = candidate.Digest
			break
		}
	}
	if environment.ModelDigest == "" {
		return Environment{}, fmt.Errorf("configured evaluation model %q is not installed", model)
	}
	return environment, nil
}

func modelMatches(installed, configured string) bool {
	if installed == configured {
		return true
	}
	return !strings.Contains(configured, ":") && installed == configured+":latest"
}

type ollamaVersionResponseTarget struct {
	environment *Environment
}

func (t *ollamaVersionResponseTarget) UnmarshalJSON(contents []byte) error {
	var response ollamaVersionResponse
	if err := json.Unmarshal(contents, &response); err != nil {
		return err
	}
	t.environment.OllamaVersion = response.Version
	return nil
}

func getJSON(ctx context.Context, client *http.Client, url string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%s returned %s", url, response.Status)
	}
	return json.NewDecoder(response.Body).Decode(target)
}
