package config

// config_test.go verifies configuration defaults, environment overrides, and validation failures.

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv(ibkrBaseURLEnv, "")
	t.Setenv(ibkrSkipTLSVerifyEnv, "")

	cfg := DefaultConfig()
	if cfg.IBKR.BaseURL != "https://localhost:5000/v1/api" {
		t.Fatalf("unexpected default IBKR URL: %s", cfg.IBKR.BaseURL)
	}
	if !cfg.IBKR.SkipTLSVerifyLocal {
		t.Fatal("local TLS verification should be disabled by default")
	}
	if cfg.AI.Provider != "ollama" || cfg.AI.BaseURL != "http://localhost:11434/api" || cfg.AI.Model != "llama3.2" {
		t.Fatalf("unexpected default AI configuration: %+v", cfg.AI)
	}
	if cfg.AI.SearchBaseURL != "https://ollama.com/api" || cfg.AI.SearchMaxResults != 5 || len(cfg.AI.SearchAllowedDomains) == 0 {
		t.Fatalf("unexpected default research configuration: %+v", cfg.AI)
	}
}

func TestLoadEnvironmentOverrides(t *testing.T) {
	t.Setenv(ibkrBaseURLEnv, "https://gateway.example.test/v1/api")
	t.Setenv(ibkrSkipTLSVerifyEnv, "false")
	t.Setenv(ibkrGatewayDirEnv, `C:\ibkr\clientportal.gw`)
	t.Setenv(aiProviderEnv, "ollama")
	t.Setenv(aiBaseURLEnv, "http://ollama.example.test:11434/api")
	t.Setenv(aiModelEnv, "gemma3")
	t.Setenv(ollamaAPIKeyEnv, "secret")
	t.Setenv(aiSearchBaseURLEnv, "https://search.example.test/api")
	t.Setenv(aiSearchDomainsEnv, "sec.gov, reuters.com")
	t.Setenv(aiSearchMaxResultsEnv, "3")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned an error: %v", err)
	}

	if cfg.IBKR.BaseURL != "https://gateway.example.test/v1/api" {
		t.Fatalf("unexpected IBKR URL: %s", cfg.IBKR.BaseURL)
	}
	if cfg.IBKR.SkipTLSVerifyLocal {
		t.Fatal("TLS verification override was not applied")
	}
	if cfg.IBKR.GatewayDirectory != `C:\ibkr\clientportal.gw` {
		t.Fatalf("unexpected gateway directory: %s", cfg.IBKR.GatewayDirectory)
	}
	if cfg.AI.Provider != "ollama" || cfg.AI.BaseURL != "http://ollama.example.test:11434/api" || cfg.AI.Model != "gemma3" {
		t.Fatalf("AI configuration overrides were not applied: %+v", cfg.AI)
	}
	if cfg.AI.SearchAPIKey != "secret" || cfg.AI.SearchBaseURL != "https://search.example.test/api" || cfg.AI.SearchMaxResults != 3 {
		t.Fatalf("research configuration overrides were not applied: %+v", cfg.AI)
	}
	if len(cfg.AI.SearchAllowedDomains) != 2 || cfg.AI.SearchAllowedDomains[1] != "reuters.com" {
		t.Fatalf("research domains were not applied: %+v", cfg.AI.SearchAllowedDomains)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	t.Run("base URL", func(t *testing.T) {
		t.Setenv(ibkrBaseURLEnv, "localhost:5000")
		t.Setenv(ibkrGatewayDirEnv, "")

		if _, err := Load(); err == nil {
			t.Fatal("expected invalid base URL to return an error")
		}
	})

	t.Run("TLS flag", func(t *testing.T) {
		t.Setenv(ibkrSkipTLSVerifyEnv, "sometimes")
		t.Setenv(ibkrGatewayDirEnv, "")

		if _, err := Load(); err == nil {
			t.Fatal("expected invalid TLS flag to return an error")
		}
	})

	t.Run("AI base URL", func(t *testing.T) {
		t.Setenv(ibkrGatewayDirEnv, "")
		t.Setenv(aiBaseURLEnv, "localhost:11434")

		if _, err := Load(); err == nil {
			t.Fatal("expected invalid AI base URL to return an error")
		}
	})

	t.Run("AI model", func(t *testing.T) {
		t.Setenv(ibkrGatewayDirEnv, "")
		t.Setenv(aiModelEnv, "")

		if _, err := Load(); err == nil {
			t.Fatal("expected empty AI model to return an error")
		}
	})

	t.Run("search result limit", func(t *testing.T) {
		t.Setenv(ibkrGatewayDirEnv, "")
		t.Setenv(aiSearchMaxResultsEnv, "11")

		if _, err := Load(); err == nil {
			t.Fatal("expected an invalid search result limit to return an error")
		}
	})
}
