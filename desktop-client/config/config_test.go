package config

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
}

func TestLoadEnvironmentOverrides(t *testing.T) {
	t.Setenv(ibkrBaseURLEnv, "https://gateway.example.test/v1/api")
	t.Setenv(ibkrSkipTLSVerifyEnv, "false")

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
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	t.Run("base URL", func(t *testing.T) {
		t.Setenv(ibkrBaseURLEnv, "localhost:5000")

		if _, err := Load(); err == nil {
			t.Fatal("expected invalid base URL to return an error")
		}
	})

	t.Run("TLS flag", func(t *testing.T) {
		t.Setenv(ibkrSkipTLSVerifyEnv, "sometimes")

		if _, err := Load(); err == nil {
			t.Fatal("expected invalid TLS flag to return an error")
		}
	})
}
