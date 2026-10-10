package ai

// This file defines privacy-constrained instrument research tools and source filtering.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// PublicInstrument is the only application-derived identity permitted in search requests.
type PublicInstrument struct {
	Name   string
	Ticker string
}

// SearchResult is one externally sourced research result.
type SearchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Content     string `json:"content"`
	RetrievedAt string `json:"retrieved_at,omitempty"`
}

// FetchedSource is the readable content of one approved search result.
type FetchedSource struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Content     string `json:"content"`
	RetrievedAt string `json:"retrieved_at,omitempty"`
}

// ResearchProvider searches and fetches public sources independently from model providers.
type ResearchProvider interface {
	Search(ctx context.Context, query string, maximum int) ([]SearchResult, error)
	Fetch(ctx context.Context, sourceURL string) (FetchedSource, error)
}

// ResearchToolsFactory creates isolated search sessions for each prompt execution.
type ResearchToolsFactory struct {
	provider       ResearchProvider
	allowedDomains []string
	maxResults     int
}

type researchSession struct {
	provider       ResearchProvider
	instruments    map[string]PublicInstrument
	allowedDomains []string
	approvedURLs   map[string]struct{}
	maxResults     int
	mu             sync.Mutex
}

type instrumentSearchTool struct {
	session *researchSession
}

type sourceFetchTool struct {
	session *researchSession
}

const (
	maxSearchSnippetCharacters = 2000
	maxFetchedSourceCharacters = 12000
)

var researchTopics = map[string]string{
	"company_risk":        "official filings business risks financial condition",
	"financial_condition": "official annual report revenue profitability cash debt",
	"fund_holdings":       "official issuer fund holdings",
	"latest_news":         "latest news earnings report financial results guidance material events",
	"recent_events":       "recent material company events",
	"regulatory_filings":  "official regulatory filings",
	"valuation":           "valuation trailing price earnings ratio market capitalization earnings per share financial results",
}

// NewResearchToolsFactory creates prompt-scoped tools with an enforced source policy.
func NewResearchToolsFactory(
	provider ResearchProvider,
	allowedDomains []string,
	maxResults int,
) *ResearchToolsFactory {
	domains := make([]string, 0, len(allowedDomains))
	for _, domain := range allowedDomains {
		if normalized := normalizeDomain(domain); normalized != "" {
			domains = append(domains, normalized)
		}
	}
	return &ResearchToolsFactory{provider: provider, allowedDomains: domains, maxResults: maxResults}
}

// ForInstruments creates tools that can search only the supplied public identities.
func (f *ResearchToolsFactory) ForInstruments(instruments []PublicInstrument) []Tool {
	allowed := make(map[string]PublicInstrument, len(instruments))
	for _, instrument := range instruments {
		instrument.Name = strings.TrimSpace(instrument.Name)
		instrument.Ticker = strings.TrimSpace(instrument.Ticker)
		if instrument.Name == "" && instrument.Ticker == "" {
			continue
		}
		allowed[instrumentKey(instrument)] = instrument
	}
	session := &researchSession{
		provider:       f.provider,
		instruments:    allowed,
		allowedDomains: append([]string(nil), f.allowedDomains...),
		approvedURLs:   make(map[string]struct{}),
		maxResults:     f.maxResults,
	}
	return []Tool{
		&instrumentSearchTool{session: session},
		&sourceFetchTool{session: session},
	}
}

func (t *instrumentSearchTool) Definition() ToolDefinition {
	topics := make([]string, 0, len(researchTopics))
	for topic := range researchTopics {
		topics = append(topics, topic)
	}
	sort.Strings(topics)
	return ToolDefinition{
		Name:        "search_instrument",
		Description: "Search approved public sources for one instrument. Only its public name, ticker, and selected research topic leave the application.",
		Parameters: map[string]any{
			"type":     "object",
			"required": []string{"name", "ticker", "topic"},
			"properties": map[string]any{
				"name":   map[string]any{"type": "string"},
				"ticker": map[string]any{"type": "string"},
				"topic":  map[string]any{"type": "string", "enum": topics},
			},
		},
	}
}

// Execute validates an instrument identity and builds the outbound query from fixed public terms.
func (t *instrumentSearchTool) Execute(ctx context.Context, arguments map[string]any) (string, error) {
	requested := PublicInstrument{
		Name:   stringArgument(arguments, "name"),
		Ticker: stringArgument(arguments, "ticker"),
	}
	instrument, ok := t.session.instruments[instrumentKey(requested)]
	if !ok {
		return "", fmt.Errorf("instrument search rejected because the name and ticker are not an allowed public identity")
	}
	topic := stringArgument(arguments, "topic")
	terms, ok := researchTopics[topic]
	if !ok {
		return "", fmt.Errorf("unsupported research topic %q", topic)
	}
	query := strings.TrimSpace(strings.Join([]string{instrument.Name, instrument.Ticker, terms}, " "))
	results, err := t.session.provider.Search(ctx, query, t.session.maxResults)
	if err != nil {
		return "", err
	}

	accepted := make([]SearchResult, 0, len(results))
	retrievedAt := time.Now().UTC().Format(time.RFC3339)
	for _, result := range results {
		if !domainAllowed(result.URL, t.session.allowedDomains) {
			continue
		}
		result.Title = limitResearchText(result.Title, 300)
		result.Content = limitResearchText(result.Content, maxSearchSnippetCharacters)
		result.RetrievedAt = retrievedAt
		accepted = append(accepted, result)
		t.session.mu.Lock()
		t.session.approvedURLs[result.URL] = struct{}{}
		t.session.mu.Unlock()
		if len(accepted) >= t.session.maxResults {
			break
		}
	}
	if len(accepted) == 0 {
		return "No results passed the configured reliable-source policy.", nil
	}
	encoded, err := json.Marshal(accepted)
	if err != nil {
		return "", fmt.Errorf("encode accepted search results: %w", err)
	}
	return string(encoded), nil
}

func (t *sourceFetchTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "fetch_source",
		Description: "Fetch the content of a reliable URL returned by search_instrument in this execution.",
		Parameters: map[string]any{
			"type":     "object",
			"required": []string{"url"},
			"properties": map[string]any{
				"url": map[string]any{"type": "string"},
			},
		},
	}
}

func (t *instrumentSearchTool) AcceptsCitation(rawURL string) bool {
	return t.session.acceptsCitation(rawURL)
}

func (t *sourceFetchTool) AcceptsCitation(rawURL string) bool {
	return t.session.acceptsCitation(rawURL)
}

func (s *researchSession) acceptsCitation(rawURL string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, accepted := s.approvedURLs[strings.TrimSpace(rawURL)]
	return accepted
}

// Execute permits fetching only URLs accepted during the same isolated search session.
func (t *sourceFetchTool) Execute(ctx context.Context, arguments map[string]any) (string, error) {
	sourceURL := stringArgument(arguments, "url")
	t.session.mu.Lock()
	_, approved := t.session.approvedURLs[sourceURL]
	t.session.mu.Unlock()
	if !approved {
		return "", fmt.Errorf("source fetch rejected because the URL was not returned by an approved search")
	}
	source, err := t.session.provider.Fetch(ctx, sourceURL)
	if err != nil {
		return "", err
	}
	source.Title = limitResearchText(source.Title, 300)
	source.Content = limitResearchText(source.Content, maxFetchedSourceCharacters)
	source.RetrievedAt = time.Now().UTC().Format(time.RFC3339)
	encoded, err := json.Marshal(source)
	if err != nil {
		return "", fmt.Errorf("encode fetched source: %w", err)
	}
	return string(encoded), nil
}

func stringArgument(arguments map[string]any, name string) string {
	value, _ := arguments[name].(string)
	return strings.TrimSpace(value)
}

func instrumentKey(instrument PublicInstrument) string {
	return strings.ToUpper(strings.TrimSpace(instrument.Ticker)) + "\x00" + strings.ToLower(strings.TrimSpace(instrument.Name))
}

// requiredInstrumentSearchCalls builds privacy-safe mandatory searches from public identities and fixed topics.
func requiredInstrumentSearchCalls(instruments []PublicInstrument, topics ...string) []ToolCall {
	calls := make([]ToolCall, 0, len(instruments)*len(topics))
	for _, instrument := range instruments {
		for _, topic := range topics {
			calls = append(calls, ToolCall{Name: "search_instrument", Arguments: map[string]any{
				"name": strings.TrimSpace(instrument.Name), "ticker": strings.TrimSpace(instrument.Ticker), "topic": topic,
			}})
		}
	}
	return calls
}

func normalizeDomain(value string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), ".")
}

func domainAllowed(rawURL string, allowedDomains []string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	for _, domain := range allowedDomains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func limitResearchText(value string, maximum int) string {
	runes := []rune(value)
	if len(runes) <= maximum {
		return value
	}
	return strings.TrimSpace(string(runes[:maximum-1])) + "…"
}
