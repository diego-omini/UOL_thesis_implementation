package ibkr

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// TODO: An abstraction layer can be introduced so that things like connectionstatus can be reused instead of being ibkr specific.

type Client struct {
	baseURL    string
	httpClient *http.Client
}

// Used to test the connection to ibkr.
type ConnectionStatus struct {
	Connected bool
	Message   string
}

// Information about authentication.
type authStatusResponse struct {
	Authenticated bool `json:"authenticated"`
	Connected     bool `json:"connected"`
	Competing     bool `json:"competing"`
}

// NewClient returns a basic client for the portal gateway.
func NewClient(baseURL string, skipTLSVerify bool) *Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: skipTLSVerify,
		},
	}

	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: transport,
		},
	}
}

// CheckConnection checks that the gateway is reachable and authenticated.
func (c *Client) CheckConnection(ctx context.Context) ConnectionStatus {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/iserver/auth/status", nil)

	if err != nil {
		return ConnectionStatus{
			Connected: false,
			Message:   fmt.Sprint("Failed to create IBKR auth status request -> %v", err),
		}
	}

	resp, err := c.httpClient.Do(req)

	if err != nil {
		return ConnectionStatus{
			Connected: false,
			Message:   fmt.Sprintf("IBKR Gateway is not reachable -> %v", err),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ConnectionStatus{
			Connected: false,
			Message:   fmt.Sprintf("IBKR gateway returned HTTP %d", resp.StatusCode),
		}
	}

	var status authStatusResponse

	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return ConnectionStatus{
			Connected: false,
			Message:   fmt.Sprintf("Failed to decode IBKR auth status response -> %v", err),
		}
	}

	if status.Competing {
		return ConnectionStatus{
			Connected: false,
			Message:   "IBKR session is competing with another active session",
		}
	}

	if status.Authenticated && status.Connected {
		return ConnectionStatus{
			Connected: true,
			Message:   "IBKR gateway is reachable and authenticated",
		}
	}

	return ConnectionStatus{
		Connected: false,
		Message:   "IBKR gateway is reachable but not authenticated",
	}
}
