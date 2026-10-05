package ai

// This file defines and validates the structured, fact-grounded counselor response contract.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

const CounselorResponseSchema = "counselor_observations_v1"

const counselorSchemaVersion = 1

const maxObservationStatementCharacters = 240

var counselorURLPattern = regexp.MustCompile(`https?://[^\s\]\)<>"']+`)

var unsafeAuthorityPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b(i|we|the counselor|this counselor|the ai|this ai)\s+(can\s+|will\s+|did\s+|have\s+)?(execute|executed|place|placed|submit|submitted|confirm|confirmed)\b`),
	regexp.MustCompile(`\b(order|trade)\s+(was|is|has been|will be)\s+(executed|placed|submitted|confirmed)\b`),
}

var unsafeAdvicePatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b(you should|i recommend|we recommend|the counselor recommends|consider)\s+(buy|buying|sell|selling|hold|holding|trim|trimming|add|adding|replace|replacing|increase|increasing|decrease|decreasing)\b`),
	regexp.MustCompile(`\b(you should|i recommend|we recommend|recommend|use|adopt|propose)\s+(an?\s+|the\s+)?target\s+(allocation|weight|position|percentage)\b`),
	regexp.MustCompile(`(^|[.!?]\s+)set\s+(an?\s+|the\s+)?target\s+(allocation|weight|position|percentage)\b`),
	regexp.MustCompile(`(^|[.!?]\s+)replace\s+[^.!?]{1,80}\s+with\s+[^.!?]{1,80}`),
	regexp.MustCompile(`\b(step\s+\d+|next|then)[:,]?\s+(place|submit|confirm)\s+(an?\s+|the\s+|this\s+)?(order|trade)\b`),
}

type ObservationCategory string

const (
	CategorySuppliedFact             ObservationCategory = "supplied_fact"
	CategoryDeterministicDerivedFact ObservationCategory = "deterministic_derived_fact"
	CategorySourcedExternalEvidence  ObservationCategory = "sourced_external_evidence"
	CategoryInterpretation           ObservationCategory = "interpretation"
	CategoryGeneralInformation       ObservationCategory = "general_information"
	CategoryUnavailableInformation   ObservationCategory = "unavailable_information"
)

type ObservationSeverity string

const (
	SeverityInformational ObservationSeverity = "informational"
	SeverityLimited       ObservationSeverity = "limited"
	SeverityModerate      ObservationSeverity = "moderate"
	SeverityElevated      ObservationSeverity = "elevated"
	SeverityHigh          ObservationSeverity = "high"
)

type ObservationConfidence string

const (
	ConfidenceLow      ObservationConfidence = "low"
	ConfidenceModerate ObservationConfidence = "moderate"
	ConfidenceHigh     ObservationConfidence = "high"
)

// CounselorFact is one application-owned fact that may support a model observation.
type CounselorFact struct {
	ID                 string   `json:"id"`
	Kind               string   `json:"kind"`
	Label              string   `json:"label"`
	TextValue          string   `json:"text_value,omitempty"`
	NumericValue       *float64 `json:"numeric_value,omitempty"`
	Unit               string   `json:"unit,omitempty"`
	Available          bool     `json:"available"`
	Limitation         string   `json:"limitation,omitempty"`
	RequiredLimitation bool     `json:"required_limitation"`
	AbsoluteTolerance  float64  `json:"absolute_tolerance,omitempty"`
	RelativeTolerance  float64  `json:"relative_tolerance,omitempty"`
}

// ReportedValue repeats one supplied numeric fact so it can be checked without model arithmetic.
type ReportedValue struct {
	FactID string  `json:"fact_id"`
	Value  float64 `json:"value"`
	Unit   string  `json:"unit"`
}

// CounselorCitation identifies external evidence accepted by the active research boundary.
type CounselorCitation struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// CounselorObservation is one machine-checkable counselor claim and its user-facing explanation.
type CounselorObservation struct {
	Category          ObservationCategory   `json:"category"`
	Statement         string                `json:"statement"`
	SupportingFactIDs []string              `json:"supporting_fact_ids"`
	ReportedValues    []ReportedValue       `json:"reported_values"`
	Severity          ObservationSeverity   `json:"severity"`
	Confidence        ObservationConfidence `json:"confidence"`
	Limitations       []string              `json:"limitations"`
	Explanation       string                `json:"explanation"`
	Citations         []CounselorCitation   `json:"citations"`
}

// CounselorLimitation records a response-level scope or data limitation and the facts it qualifies.
type CounselorLimitation struct {
	Statement string   `json:"statement"`
	FactIDs   []string `json:"fact_ids"`
}

// CounselorResponse is the application-owned structured result returned to the frontend.
type CounselorResponse struct {
	SchemaVersion int                    `json:"schema_version"`
	Summary       string                 `json:"summary"`
	Observations  []CounselorObservation `json:"observations"`
	Limitations   []CounselorLimitation  `json:"limitations"`
}

// CounselorValidationResult retains the errors from each validation pass.
type CounselorValidationResult struct {
	Valid         bool     `json:"valid"`
	InitialErrors []string `json:"initial_errors"`
	FinalErrors   []string `json:"final_errors"`
}

type citationAcceptor interface {
	AcceptsCitation(string) bool
}

// counselorJSONSchema constrains local providers while Go remains the authoritative validator.
func counselorJSONSchema() map[string]any {
	stringArray := map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	citation := strictObject(map[string]any{
		"title": map[string]any{"type": "string"},
		"url":   map[string]any{"type": "string"},
	}, []string{"title", "url"})
	reportedValue := strictObject(map[string]any{
		"fact_id": map[string]any{"type": "string"},
		"value":   map[string]any{"type": "number"},
		"unit":    map[string]any{"type": "string"},
	}, []string{"fact_id", "value", "unit"})
	observation := strictObject(map[string]any{
		"category": map[string]any{"type": "string", "enum": []string{
			string(CategorySuppliedFact), string(CategoryDeterministicDerivedFact), string(CategorySourcedExternalEvidence),
			string(CategoryInterpretation), string(CategoryGeneralInformation), string(CategoryUnavailableInformation),
		}},
		"statement":           map[string]any{"type": "string", "maxLength": maxObservationStatementCharacters},
		"supporting_fact_ids": stringArray,
		"reported_values":     map[string]any{"type": "array", "items": reportedValue},
		"severity": map[string]any{"type": "string", "enum": []string{
			string(SeverityInformational), string(SeverityLimited), string(SeverityModerate), string(SeverityElevated), string(SeverityHigh),
		}},
		"confidence": map[string]any{"type": "string", "enum": []string{
			string(ConfidenceLow), string(ConfidenceModerate), string(ConfidenceHigh),
		}},
		"limitations": stringArray,
		"explanation": map[string]any{"type": "string"},
		"citations":   map[string]any{"type": "array", "items": citation},
	}, []string{"category", "statement", "supporting_fact_ids", "reported_values", "severity", "confidence", "limitations", "explanation", "citations"})
	limitation := strictObject(map[string]any{
		"statement": map[string]any{"type": "string"},
		"fact_ids":  stringArray,
	}, []string{"statement", "fact_ids"})
	return strictObject(map[string]any{
		"schema_version": map[string]any{"type": "integer", "const": counselorSchemaVersion},
		"summary":        map[string]any{"type": "string"},
		"observations":   map[string]any{"type": "array", "items": observation, "minItems": 1},
		"limitations":    map[string]any{"type": "array", "items": limitation, "minItems": 1},
	}, []string{"schema_version", "summary", "observations", "limitations"})
}

func strictObject(properties map[string]any, required []string) map[string]any {
	return map[string]any{
		"type": "object", "properties": properties, "required": required, "additionalProperties": false,
	}
}

// decodeAndValidateCounselorResponse rejects structural and semantic grounding failures.
func decodeAndValidateCounselorResponse(text string, facts []CounselorFact, tools map[string]Tool) (*CounselorResponse, []string) {
	decoder := json.NewDecoder(bytes.NewBufferString(strings.TrimSpace(text)))
	decoder.DisallowUnknownFields()
	var response CounselorResponse
	if err := decoder.Decode(&response); err != nil {
		return nil, []string{fmt.Sprintf("decode structured counselor response: %v", err)}
	}
	if err := ensureCounselorJSONEnd(decoder); err != nil {
		return nil, []string{err.Error()}
	}
	normalizeCounselorResponse(&response, facts, tools)
	errors := validateCounselorResponse(response, facts, tools)
	if len(errors) > 0 {
		return nil, errors
	}
	return &response, nil
}

// DecodeStoredCounselorResponse accepts only a complete, safe structured response from a previously validated execution.
func DecodeStoredCounselorResponse(text string) (*CounselorResponse, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(strings.TrimSpace(text)))
	decoder.DisallowUnknownFields()
	var response CounselorResponse
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("decode stored counselor response: %w", err)
	}
	if err := ensureCounselorJSONEnd(decoder); err != nil {
		return nil, err
	}
	if response.SchemaVersion != counselorSchemaVersion || strings.TrimSpace(response.Summary) == "" || len(response.Observations) == 0 || len(response.Limitations) == 0 {
		return nil, fmt.Errorf("stored counselor response is incomplete")
	}
	allText := []string{response.Summary}
	for index, observation := range response.Observations {
		if !validCategory(observation.Category) || !validSeverity(observation.Severity) || !validConfidence(observation.Confidence) || strings.TrimSpace(observation.Statement) == "" || strings.TrimSpace(observation.Explanation) == "" {
			return nil, fmt.Errorf("stored counselor observation %d is invalid", index)
		}
		allText = append(allText, observation.Statement, observation.Explanation, strings.Join(observation.Limitations, " "))
		for _, citation := range observation.Citations {
			parsed, err := url.Parse(strings.TrimSpace(citation.URL))
			if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
				return nil, fmt.Errorf("stored counselor observation %d has an invalid citation", index)
			}
		}
	}
	for index, limitation := range response.Limitations {
		if strings.TrimSpace(limitation.Statement) == "" {
			return nil, fmt.Errorf("stored counselor limitation %d is empty", index)
		}
		allText = append(allText, limitation.Statement)
	}
	joined := strings.Join(allText, " ")
	if unsafeAuthorityClaim(joined) != "" || unsafeAdviceClaim(joined) != "" {
		return nil, fmt.Errorf("stored counselor response violates the read-only safety boundary")
	}
	return &response, nil
}

// normalizeCounselorResponse repairs representation-only mistakes without changing claims or values.
func normalizeCounselorResponse(response *CounselorResponse, facts []CounselorFact, tools map[string]Tool) {
	if response == nil {
		return
	}
	factByID := make(map[string]CounselorFact, len(facts))
	for _, fact := range facts {
		factByID[fact.ID] = fact
	}
	observations := make([]CounselorObservation, 0, len(response.Observations))
	for index := range response.Observations {
		observation := &response.Observations[index]
		if strings.TrimSpace(observation.Statement) == "" || strings.TrimSpace(observation.Explanation) == "" {
			continue
		}
		observation.SupportingFactIDs = normalizeFactIDs(observation.SupportingFactIDs)
		reportedValues := make([]ReportedValue, 0, len(observation.ReportedValues))
		for valueIndex := range observation.ReportedValues {
			value := observation.ReportedValues[valueIndex]
			value.FactID = normalizeFactID(value.FactID)
			fact, known := factByID[value.FactID]
			if !known || !fact.Available || fact.NumericValue == nil {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(value.Unit)) {
			case "%", "percentage", "percentages":
				value.Unit = "percent"
			case "percentage point", "percentage points", "percentage_point":
				value.Unit = "percentage_points"
			}
			if value.FactID != "" && !containsString(observation.SupportingFactIDs, value.FactID) {
				observation.SupportingFactIDs = append(observation.SupportingFactIDs, value.FactID)
			}
			reportedValues = append(reportedValues, value)
		}
		observation.ReportedValues = reportedValues
		normalizeObservationCategory(observation, factByID, tools)
		observations = append(observations, *observation)
	}
	response.Observations = observations
	coveredLimitations := map[string]bool{}
	for index := range response.Limitations {
		factIDs := normalizeFactIDs(response.Limitations[index].FactIDs)
		response.Limitations[index].FactIDs = response.Limitations[index].FactIDs[:0]
		for _, factID := range factIDs {
			if _, known := factByID[factID]; known {
				response.Limitations[index].FactIDs = append(response.Limitations[index].FactIDs, factID)
			}
		}
		for _, factID := range response.Limitations[index].FactIDs {
			coveredLimitations[factID] = true
		}
	}
	for _, fact := range facts {
		if fact.RequiredLimitation && !coveredLimitations[fact.ID] {
			statement := strings.TrimSpace(fact.Limitation)
			if statement == "" {
				statement = fact.Label + " is unavailable or incomplete."
			}
			response.Limitations = append(response.Limitations, CounselorLimitation{Statement: statement, FactIDs: []string{fact.ID}})
		}
	}
	if len(response.Limitations) == 0 {
		response.Limitations = []CounselorLimitation{{Statement: "This explanation is limited to the application-supplied facts and their timestamps.", FactIDs: []string{}}}
	}
}

func normalizeObservationCategory(observation *CounselorObservation, facts map[string]CounselorFact, tools map[string]Tool) {
	if observation == nil || len(observation.SupportingFactIDs) == 0 {
		return
	}
	hasSupplied := false
	hasDeterministic := false
	hasUnavailable := false
	for _, factID := range observation.SupportingFactIDs {
		fact, ok := facts[factID]
		if !ok {
			continue
		}
		switch ObservationCategory(fact.Kind) {
		case CategorySuppliedFact:
			hasSupplied = true
		case CategoryDeterministicDerivedFact:
			hasDeterministic = true
		case CategoryUnavailableInformation:
			hasUnavailable = true
		}
	}
	if hasUnavailable {
		observation.Category = CategoryUnavailableInformation
		return
	}
	if observation.Category == CategorySourcedExternalEvidence && (len(tools) == 0 || len(observation.Citations) == 0) {
		observation.Citations = nil
		observation.Category = CategoryInterpretation
	}
	if observation.Category != CategorySuppliedFact && observation.Category != CategoryDeterministicDerivedFact && observation.Category != CategoryUnavailableInformation {
		return
	}
	if hasSupplied && !hasDeterministic {
		observation.Category = CategorySuppliedFact
	} else if hasDeterministic && !hasSupplied {
		observation.Category = CategoryDeterministicDerivedFact
	} else {
		observation.Category = CategoryInterpretation
	}
}

func normalizeFactIDs(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = normalizeFactID(value)
		if value != "" && !containsString(result, value) {
			result = append(result, value)
		}
	}
	return result
}

func normalizeFactID(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "id=")
	if separator := strings.Index(value, " |"); separator >= 0 {
		value = value[:separator]
	}
	return strings.TrimSpace(value)
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func ensureCounselorJSONEnd(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("structured counselor response contains trailing JSON")
		}
		return fmt.Errorf("decode structured counselor response suffix: %w", err)
	}
	return nil
}

// validateCounselorResponse checks claim support, numeric agreement, limitations, citations, and authority safety.
func validateCounselorResponse(response CounselorResponse, facts []CounselorFact, tools map[string]Tool) []string {
	errors := []string{}
	if response.SchemaVersion != counselorSchemaVersion {
		errors = append(errors, fmt.Sprintf("schema_version must be %d", counselorSchemaVersion))
	}
	if strings.TrimSpace(response.Summary) == "" {
		errors = append(errors, "summary is required")
	}
	if len(response.Observations) == 0 {
		errors = append(errors, "at least one observation is required")
	}
	if len(response.Limitations) == 0 {
		errors = append(errors, "at least one response limitation is required")
	}

	factByID := make(map[string]CounselorFact, len(facts))
	for _, fact := range facts {
		factByID[fact.ID] = fact
	}
	coveredLimitations := map[string]bool{}
	for index, limitation := range response.Limitations {
		if strings.TrimSpace(limitation.Statement) == "" {
			errors = append(errors, fmt.Sprintf("limitation %d has an empty statement", index))
		}
		for _, factID := range limitation.FactIDs {
			if _, ok := factByID[factID]; !ok {
				errors = append(errors, fmt.Sprintf("limitation %d references unknown fact %q", index, factID))
				continue
			}
			coveredLimitations[factID] = true
		}
	}
	for _, fact := range facts {
		if fact.RequiredLimitation && !coveredLimitations[fact.ID] {
			errors = append(errors, fmt.Sprintf("required limitation for fact %q is missing", fact.ID))
		}
	}

	allText := []string{response.Summary}
	for index, observation := range response.Observations {
		allText = append(allText, observation.Statement, observation.Explanation, strings.Join(observation.Limitations, " "))
		errors = append(errors, validateObservation(index, observation, factByID, tools)...)
	}
	for _, limitation := range response.Limitations {
		allText = append(allText, limitation.Statement)
	}
	for _, rawURL := range counselorURLPattern.FindAllString(strings.Join(allText, " "), -1) {
		if !acceptedCitation(rawURL, tools) {
			errors = append(errors, fmt.Sprintf("response text contains URL outside the accepted research boundary: %q", rawURL))
		}
	}
	if unsafe := unsafeAuthorityClaim(strings.Join(allText, " ")); unsafe != "" {
		errors = append(errors, fmt.Sprintf("response contains unsupported trading authority or execution claim %q", unsafe))
	}
	if unsafe := unsafeAdviceClaim(strings.Join(allText, " ")); unsafe != "" {
		errors = append(errors, fmt.Sprintf("response contains prohibited prescriptive or order-like guidance %q", unsafe))
	}
	return errors
}

// validateObservation enforces the category-specific support and reporting rules for one claim.
func validateObservation(index int, observation CounselorObservation, facts map[string]CounselorFact, tools map[string]Tool) []string {
	errors := []string{}
	if !validCategory(observation.Category) {
		errors = append(errors, fmt.Sprintf("observation %d has invalid category %q", index, observation.Category))
	}
	if !validSeverity(observation.Severity) {
		errors = append(errors, fmt.Sprintf("observation %d has invalid severity %q", index, observation.Severity))
	}
	if !validConfidence(observation.Confidence) {
		errors = append(errors, fmt.Sprintf("observation %d has invalid confidence %q", index, observation.Confidence))
	}
	if strings.TrimSpace(observation.Statement) == "" || strings.TrimSpace(observation.Explanation) == "" {
		errors = append(errors, fmt.Sprintf("observation %d requires a statement and explanation", index))
	}
	if utf8.RuneCountInString(observation.Statement) > maxObservationStatementCharacters {
		errors = append(errors, fmt.Sprintf("observation %d statement exceeds %d characters", index, maxObservationStatementCharacters))
	}

	referenced := map[string]CounselorFact{}
	for _, factID := range observation.SupportingFactIDs {
		fact, ok := facts[factID]
		if !ok {
			errors = append(errors, fmt.Sprintf("observation %d references unknown fact %q", index, factID))
			continue
		}
		referenced[factID] = fact
	}
	if categoryRequiresSupport(observation.Category) && len(observation.SupportingFactIDs) == 0 && len(observation.Citations) == 0 {
		errors = append(errors, fmt.Sprintf("observation %d has no supporting fact or citation", index))
	}
	if observation.Category == CategorySourcedExternalEvidence && len(observation.Citations) == 0 {
		errors = append(errors, fmt.Sprintf("observation %d requires an accepted citation", index))
	}
	if observation.Category == CategoryUnavailableInformation {
		if len(referenced) > 0 && !referencesUnavailableFact(referenced) {
			errors = append(errors, fmt.Sprintf("observation %d must reference unavailable information", index))
		}
		if len(referenced) == 0 && len(observation.Limitations) == 0 {
			errors = append(errors, fmt.Sprintf("observation %d must explain the unavailable evidence", index))
		}
	}
	if observation.Category == CategorySuppliedFact && !allFactsHaveKind(referenced, CategorySuppliedFact) {
		errors = append(errors, fmt.Sprintf("observation %d labels non-supplied support as a supplied fact", index))
	}
	if observation.Category == CategoryDeterministicDerivedFact && !allFactsHaveKind(referenced, CategoryDeterministicDerivedFact) {
		errors = append(errors, fmt.Sprintf("observation %d labels non-deterministic support as a deterministic fact", index))
	}
	for _, value := range observation.ReportedValues {
		fact, ok := facts[value.FactID]
		if !ok {
			errors = append(errors, fmt.Sprintf("observation %d reports unknown fact %q", index, value.FactID))
			continue
		}
		if _, supported := referenced[value.FactID]; !supported {
			errors = append(errors, fmt.Sprintf("observation %d reports fact %q without listing it as support", index, value.FactID))
		}
		if fact.NumericValue == nil || !fact.Available {
			errors = append(errors, fmt.Sprintf("observation %d reports a numeric value for unavailable or non-numeric fact %q", index, value.FactID))
			continue
		}
		if value.Unit != fact.Unit {
			errors = append(errors, fmt.Sprintf("observation %d reports unit %q for fact %q; expected %q", index, value.Unit, value.FactID, fact.Unit))
			continue
		}
		tolerance := math.Max(fact.AbsoluteTolerance, math.Abs(*fact.NumericValue)*fact.RelativeTolerance)
		if math.IsNaN(value.Value) || math.IsInf(value.Value, 0) || math.Abs(value.Value-*fact.NumericValue) > tolerance {
			errors = append(errors, fmt.Sprintf("observation %d reports %.10g for fact %q; supplied value is %.10g %s with tolerance %.10g", index, value.Value, value.FactID, *fact.NumericValue, fact.Unit, tolerance))
		}
	}
	for _, citation := range observation.Citations {
		if !acceptedCitation(citation.URL, tools) {
			errors = append(errors, fmt.Sprintf("observation %d cites URL outside the accepted research boundary: %q", index, citation.URL))
		}
	}
	return errors
}

func acceptedCitation(rawURL string, tools map[string]Tool) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return false
	}
	for _, tool := range tools {
		if acceptor, ok := tool.(citationAcceptor); ok && acceptor.AcceptsCitation(rawURL) {
			return true
		}
	}
	return false
}

func validCategory(value ObservationCategory) bool {
	switch value {
	case CategorySuppliedFact, CategoryDeterministicDerivedFact, CategorySourcedExternalEvidence,
		CategoryInterpretation, CategoryGeneralInformation, CategoryUnavailableInformation:
		return true
	default:
		return false
	}
}

func validSeverity(value ObservationSeverity) bool {
	switch value {
	case SeverityInformational, SeverityLimited, SeverityModerate, SeverityElevated, SeverityHigh:
		return true
	default:
		return false
	}
}

func validConfidence(value ObservationConfidence) bool {
	return value == ConfidenceLow || value == ConfidenceModerate || value == ConfidenceHigh
}

func categoryRequiresSupport(category ObservationCategory) bool {
	return category != CategoryGeneralInformation && category != CategoryUnavailableInformation
}

func referencesUnavailableFact(facts map[string]CounselorFact) bool {
	for _, fact := range facts {
		if !fact.Available || fact.Kind == string(CategoryUnavailableInformation) {
			return true
		}
	}
	return false
}

func allFactsHaveKind(facts map[string]CounselorFact, kind ObservationCategory) bool {
	if len(facts) == 0 {
		return false
	}
	for _, fact := range facts {
		if fact.Kind != string(kind) {
			return false
		}
	}
	return true
}

func unsafeAuthorityClaim(text string) string {
	normalized := strings.ToLower(strings.Join(strings.Fields(text), " "))
	for _, pattern := range unsafeAuthorityPatterns {
		if match := pattern.FindString(normalized); match != "" {
			return match
		}
	}
	return ""
}

func unsafeAdviceClaim(text string) string {
	normalized := strings.ToLower(strings.Join(strings.Fields(text), " "))
	for _, pattern := range unsafeAdvicePatterns {
		if match := pattern.FindString(normalized); match != "" {
			return match
		}
	}
	return ""
}
