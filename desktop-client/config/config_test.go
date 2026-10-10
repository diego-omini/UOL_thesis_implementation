package config

// config_test.go verifies configuration defaults, environment overrides, and validation failures.

import (
	"os"
	"testing"
)

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
	if cfg.AI.Provider != "ollama" || cfg.AI.BaseURL != "http://localhost:11434/api" || cfg.AI.Model != "qwen3:14b" {
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
	t.Setenv(snapTradeClientIDEnv, "personal-client")
	t.Setenv(snapTradeConsumerEnv, "personal-secret")

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
	if cfg.SnapTrade.ClientID != "personal-client" || cfg.SnapTrade.ConsumerKey != "personal-secret" {
		t.Fatalf("SnapTrade configuration overrides were not applied")
	}
}

func TestLoadOpenAIProviderDefaults(t *testing.T) {
	t.Setenv(ibkrGatewayDirEnv, "")
	t.Setenv(aiProviderEnv, "openai")
	t.Setenv(openAIAPIKeyEnv, "test-key")
	osUnsetForTest(t, aiBaseURLEnv)
	osUnsetForTest(t, aiModelEnv)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned an error: %v", err)
	}
	if cfg.AI.Provider != "openai" || cfg.AI.BaseURL != "https://api.openai.com/v1" || cfg.AI.Model != "gpt-6-luna" {
		t.Fatalf("unexpected OpenAI defaults: %+v", cfg.AI)
	}
	if cfg.AI.APIKey != "test-key" {
		t.Fatal("OpenAI API key was not loaded")
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

	t.Run("OpenAI API key", func(t *testing.T) {
		t.Setenv(ibkrGatewayDirEnv, "")
		t.Setenv(aiProviderEnv, "openai")
		t.Setenv(openAIAPIKeyEnv, "")
		osUnsetForTest(t, aiBaseURLEnv)
		osUnsetForTest(t, aiModelEnv)

		if _, err := Load(); err == nil {
			t.Fatal("expected an empty OpenAI API key to return an error")
		}
	})

	t.Run("AI provider", func(t *testing.T) {
		t.Setenv(ibkrGatewayDirEnv, "")
		t.Setenv(aiProviderEnv, "unsupported")

		if _, err := Load(); err == nil {
			t.Fatal("expected an unsupported AI provider to return an error")
		}
	})

	t.Run("search result limit", func(t *testing.T) {
		t.Setenv(ibkrGatewayDirEnv, "")
		t.Setenv(aiSearchMaxResultsEnv, "11")

		if _, err := Load(); err == nil {
			t.Fatal("expected an invalid search result limit to return an error")
		}
	})

	t.Run("partial SnapTrade credentials", func(t *testing.T) {
		t.Setenv(ibkrGatewayDirEnv, "")
		t.Setenv(snapTradeClientIDEnv, "personal-client")
		t.Setenv(snapTradeConsumerEnv, "")

		if _, err := Load(); err == nil {
			t.Fatal("expected partial SnapTrade credentials to return an error")
		}
	})
}

func osUnsetForTest(t *testing.T, name string) {
	t.Helper()
	value, exists := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("unset %s: %v", name, err)
	}
	t.Cleanup(func() {
		if exists {
			_ = os.Setenv(name, value)
		} else {
			_ = os.Unsetenv(name)
		}
	})
}
