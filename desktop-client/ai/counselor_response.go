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
	"sort"
	"strings"
	"unicode/utf8"
)

const CounselorResponseSchema = "counselor_observations_v1"

const counselorSchemaVersion = 1

const maxObservationStatementCharacters = 240

const (
	maxCounselorObservations         = 12
	maxCounselorLimitations          = 24
	maxSupportingFactsPerObservation = 16
	maxReportedValuesPerObservation  = 10
	maxCitationsPerObservation       = 5
	maxObservationLimitations        = 5
	maxSummaryCharacters             = 1200
	maxExplanationCharacters         = 1600
)

var counselorURLPattern = regexp.MustCompile(`https?://[^\s\]\)<>"']+`)

var unsafeAuthorityPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b(i|we|the counselor|this counselor|the ai|this ai)\s+(can\s+|will\s+|did\s+|have\s+)?(execute|executed|place|placed|submit|submitted|confirm|confirmed)\b`),
	regexp.MustCompile(`\b(i|we|the counselor|this counselor|the ai|this ai)\s+(can\s+|will\s+|did\s+|have\s+)?(trade|traded|transact|transacted|rebalance|rebalanced|liquidate|liquidated)\b`),
	regexp.MustCompile(`\b(order|trade)\s+(was|is|has been|will be)\s+(executed|placed|submitted|confirmed)\b`),
	regexp.MustCompile(`\b(order|trade)\s+(was|is|has been|will be)\s+(sent|transmitted)\b`),
	regexp.MustCompile(`\b(i|we|the counselor|this counselor|the ai|this ai)\s+(can\s+|do\s+|will\s+|have\s+)?(access|control|modify|manage)\s+(your\s+|the\s+)?(broker|brokerage|account|portfolio)\b`),
}

var unsafeAdvicePatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b(you should|you must|you need to|you may want to|i recommend|i advise|we recommend|we advise|the counselor recommends|consider)\s+(not\s+)?(buy|buying|sell|selling|hold|holding|trim|trimming|add|adding|replace|replacing|increase|increasing|decrease|decreasing|exit|exiting|liquidate|liquidating|rebalance|rebalancing)\b`),
	regexp.MustCompile(`\b(it is|it would be)\s+(advisable|prudent|best|appropriate)\s+to\s+(buy|sell|hold|trim|add|replace|increase|decrease|exit|liquidate|rebalance)\b`),
	regexp.MustCompile(`\b(this|the)\s+(stock|fund|security|position|order)\s+(is|looks|appears)\s+(like\s+)?(an?\s+)?(buy|sell|hold)\b`),
	regexp.MustCompile(`\b(you should|i recommend|we recommend|recommend|use|adopt|propose)\s+(an?\s+|the\s+)?target\s+(allocation|weight|position|percentage)\b`),
	regexp.MustCompile(`(^|[.!?]\s+)set\s+(an?\s+|the\s+)?target\s+(allocation|weight|position|percentage)\b`),
	regexp.MustCompile(`(^|[.!?]\s+)replace\s+[^.!?]{1,80}\s+with\s+[^.!?]{1,80}`),
	regexp.MustCompile(`\b(step\s+\d+|next|then)[:,]?\s+(place|submit|confirm)\s+(an?\s+|the\s+|this\s+)?(order|trade)\b`),
	regexp.MustCompile(`(^|[.!?]\s+)(buy|sell|trim|add|reduce|replace)\s+[^.!?]{1,120}`),
	regexp.MustCompile(`\b(allocate|move|shift|rotate|switch)\s+[^.!?]{0,100}\s+(to|into)\s+[^.!?]{1,100}`),
	regexp.MustCompile(`\b(target|set|reduce|increase)\s+[^.!?]{0,80}\s+(to|at)\s+-?\d+(?:\.\d+)?\s*%`),
	regexp.MustCompile(`\b(keep|cap|limit)\s+[^.!?]{0,80}\s+(to|at|below|under)\s+-?\d+(?:\.\d+)?\s*%`),
	regexp.MustCompile(`(^|[.!?]\s+)(avoid|do not)\s+(buying|selling|holding|adding|trimming|replacing|increasing|decreasing)\b`),
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
	ID                  string   `json:"id"`
	Kind                string   `json:"kind"`
	Label               string   `json:"label"`
	TextValue           string   `json:"text_value,omitempty"`
	NumericValue        *float64 `json:"numeric_value,omitempty"`
	Unit                string   `json:"unit,omitempty"`
	Available           bool     `json:"available"`
	Limitation          string   `json:"limitation,omitempty"`
	RequiredLimitation  bool     `json:"required_limitation"`
	RequiredObservation bool     `json:"required_observation"`
	ObservationOrder    int      `json:"observation_order,omitempty"`
	AbsoluteTolerance   float64  `json:"absolute_tolerance,omitempty"`
	RelativeTolerance   float64  `json:"relative_tolerance,omitempty"`
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
func counselorJSONSchema(maximumObservations int) map[string]any {
	maximumObservations = effectiveMaxObservations(maximumObservations)
	nonEmptyString := map[string]any{"type": "string", "minLength": 1}
	stringArray := map[string]any{"type": "array", "items": nonEmptyString}
	citation := strictObject(map[string]any{
		"title": nonEmptyString,
		"url":   nonEmptyString,
	}, []string{"title", "url"})
	reportedValue := strictObject(map[string]any{
		"fact_id": nonEmptyString,
		"value":   map[string]any{"type": "number"},
		"unit":    nonEmptyString,
	}, []string{"fact_id", "value", "unit"})
	observation := strictObject(map[string]any{
		"category": map[string]any{"type": "string", "enum": []string{
			string(CategorySuppliedFact), string(CategoryDeterministicDerivedFact), string(CategorySourcedExternalEvidence),
			string(CategoryInterpretation), string(CategoryGeneralInformation), string(CategoryUnavailableInformation),
		}},
		"statement":           map[string]any{"type": "string", "minLength": 1, "maxLength": maxObservationStatementCharacters},
		"supporting_fact_ids": map[string]any{"type": "array", "items": nonEmptyString, "maxItems": maxSupportingFactsPerObservation},
		"reported_values":     map[string]any{"type": "array", "items": reportedValue, "maxItems": maxReportedValuesPerObservation},
		"severity": map[string]any{"type": "string", "enum": []string{
			string(SeverityInformational), string(SeverityLimited), string(SeverityModerate), string(SeverityElevated), string(SeverityHigh),
		}},
		"confidence": map[string]any{"type": "string", "enum": []string{
			string(ConfidenceLow), string(ConfidenceModerate), string(ConfidenceHigh),
		}},
		"limitations": map[string]any{"type": "array", "items": nonEmptyString, "maxItems": maxObservationLimitations},
		"explanation": map[string]any{"type": "string", "minLength": 1, "maxLength": maxExplanationCharacters},
		"citations":   map[string]any{"type": "array", "items": citation, "maxItems": maxCitationsPerObservation},
	}, []string{"category", "statement", "supporting_fact_ids", "reported_values", "severity", "confidence", "limitations", "explanation", "citations"})
	limitation := strictObject(map[string]any{
		"statement": nonEmptyString,
		"fact_ids":  stringArray,
	}, []string{"statement", "fact_ids"})
	return strictObject(map[string]any{
		"schema_version": map[string]any{"type": "integer", "const": counselorSchemaVersion},
		"summary":        map[string]any{"type": "string", "minLength": 1, "maxLength": maxSummaryCharacters},
		"observations":   map[string]any{"type": "array", "items": observation, "minItems": 1, "maxItems": maximumObservations},
		"limitations":    map[string]any{"type": "array", "items": limitation, "minItems": 1, "maxItems": maxCounselorLimitations},
	}, []string{"schema_version", "summary", "observations", "limitations"})
}

func strictObject(properties map[string]any, required []string) map[string]any {
	return map[string]any{
		"type": "object", "properties": properties, "required": required, "additionalProperties": false,
	}
}

// decodeAndValidateCounselorResponse rejects structural and semantic grounding failures.
func decodeAndValidateCounselorResponse(text string, facts []CounselorFact, tools map[string]Tool) (*CounselorResponse, []string) {
	return decodeAndValidateCounselorResponseWithLimit(text, facts, tools, maxCounselorObservations)
}

func decodeAndValidateCounselorResponseWithLimit(text string, facts []CounselorFact, tools map[string]Tool, maximumObservations int) (*CounselorResponse, []string) {
	decoder := json.NewDecoder(bytes.NewBufferString(strings.TrimSpace(text)))
	decoder.DisallowUnknownFields()
	var response CounselorResponse
	if err := decoder.Decode(&response); err != nil {
		return nil, []string{fmt.Sprintf("decode structured counselor response: %v", err)}
	}
	if err := ensureCounselorJSONEnd(decoder); err != nil {
		return nil, []string{err.Error()}
	}
	normalizeCounselorResponse(&response, facts)
	errors := validateCounselorResponse(response, facts, tools, maximumObservations)
	if len(errors) > 0 {
		return nil, errors
	}
	return &response, nil
}

// decodeCounselorResponseForDisplay reads recognizable schema-shaped JSON without treating it as validated.
func decodeCounselorResponseForDisplay(text string) *CounselorResponse {
	candidate := counselorJSONDisplayCandidate(text)
	if candidate == "" {
		return nil
	}
	var response CounselorResponse
	if err := json.Unmarshal([]byte(candidate), &response); err != nil {
		return nil
	}
	if strings.TrimSpace(response.Summary) == "" && len(response.Observations) == 0 && len(response.Limitations) == 0 {
		return nil
	}
	if response.Observations == nil {
		response.Observations = []CounselorObservation{}
	}
	if response.Limitations == nil {
		response.Limitations = []CounselorLimitation{}
	}
	for index := range response.Observations {
		observation := &response.Observations[index]
		if observation.SupportingFactIDs == nil {
			observation.SupportingFactIDs = []string{}
		}
		if observation.ReportedValues == nil {
			observation.ReportedValues = []ReportedValue{}
		}
		if observation.Limitations == nil {
			observation.Limitations = []string{}
		}
		if observation.Citations == nil {
			observation.Citations = []CounselorCitation{}
		}
	}
	for index := range response.Limitations {
		if response.Limitations[index].FactIDs == nil {
			response.Limitations[index].FactIDs = []string{}
		}
	}
	return &response
}

// counselorJSONDisplayCandidate accepts plain JSON or a complete JSON Markdown fence.
func counselorJSONDisplayCandidate(text string) string {
	trimmed := strings.TrimSpace(text)
	if json.Valid([]byte(trimmed)) {
		return trimmed
	}
	for remainder := trimmed; ; {
		opening := strings.Index(remainder, "```")
		if opening < 0 {
			return ""
		}
		remainder = remainder[opening+3:]
		lineEnd := strings.IndexByte(remainder, '\n')
		if lineEnd < 0 {
			return ""
		}
		body := remainder[lineEnd+1:]
		closing := strings.Index(body, "```")
		if closing < 0 {
			return ""
		}
		candidate := strings.TrimSpace(body[:closing])
		if json.Valid([]byte(candidate)) {
			return candidate
		}
		remainder = body[closing+3:]
	}
}

// salvageCounselorResponse converts a structurally readable but imperfect model response into a safe,
// grounded response. It is used only after the normal validation and one model repair attempt fail.
func salvageCounselorResponse(text string, facts []CounselorFact, tools map[string]Tool, maximumObservations, maximumCharacters int) (string, *CounselorResponse, error) {
	var response CounselorResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &response); err != nil {
		response = fallbackCounselorResponse(facts, maximumObservations)
	} else {
		response = sanitizeCounselorResponse(response, facts, tools, maximumObservations)
	}
	normalizeCounselorResponse(&response, facts)
	appendSalvageLimitation(&response)
	compactCounselorLimitations(&response)
	if validationErrors := validateCounselorResponse(response, facts, tools, maximumObservations); len(validationErrors) > 0 {
		response = fallbackCounselorResponse(facts, maximumObservations)
		normalizeCounselorResponse(&response, facts)
		appendSalvageLimitation(&response)
		compactCounselorLimitations(&response)
		if fallbackErrors := validateCounselorResponse(response, facts, tools, maximumObservations); len(fallbackErrors) > 0 {
			return "", nil, fmt.Errorf("construct safe counselor fallback: %s", strings.Join(fallbackErrors, "; "))
		}
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		return "", nil, fmt.Errorf("encode salvaged counselor response: %w", err)
	}
	if maximumCharacters > 0 && utf8.RuneCount(encoded) > maximumCharacters {
		response.Summary = truncateCounselorText(response.Summary, 240)
		for index := range response.Observations {
			response.Observations[index].Statement = truncateCounselorText(response.Observations[index].Statement, 160)
			response.Observations[index].Explanation = truncateCounselorText(response.Observations[index].Explanation, 320)
			if len(response.Observations[index].Limitations) > 1 {
				response.Observations[index].Limitations = response.Observations[index].Limitations[:1]
			}
		}
		encoded, err = json.Marshal(response)
		if err != nil {
			return "", nil, fmt.Errorf("encode compact salvaged counselor response: %w", err)
		}
		if utf8.RuneCount(encoded) > maximumCharacters {
			return "", nil, fmt.Errorf("safe counselor fallback exceeds the %d-character limit", maximumCharacters)
		}
	}
	return string(encoded), &response, nil
}

func sanitizeCounselorResponse(response CounselorResponse, facts []CounselorFact, tools map[string]Tool, maximumObservations int) CounselorResponse {
	factByID := make(map[string]CounselorFact, len(facts))
	requiredFacts := make([]CounselorFact, 0)
	orderedFacts := make([]CounselorFact, 0)
	for _, fact := range facts {
		factByID[fact.ID] = fact
		if fact.RequiredObservation {
			requiredFacts = append(requiredFacts, fact)
		}
		if fact.ObservationOrder > 0 {
			orderedFacts = append(orderedFacts, fact)
		}
	}
	sort.SliceStable(orderedFacts, func(left, right int) bool {
		return orderedFacts[left].ObservationOrder < orderedFacts[right].ObservationOrder
	})

	response.SchemaVersion = counselorSchemaVersion
	response.Summary = sanitizeCounselorText(response.Summary)
	if response.Summary == "" || unsafeAuthorityClaim(response.Summary) != "" || unsafeAdviceClaim(response.Summary) != "" {
		response.Summary = "The usable parts of the model response were retained and checked against the available evidence."
	}
	response.Summary = truncateCounselorText(response.Summary, maxSummaryCharacters)

	cleaned := make([]CounselorObservation, 0, len(response.Observations))
	for _, observation := range response.Observations {
		observation.Statement = sanitizeCounselorText(observation.Statement)
		observation.Explanation = sanitizeCounselorText(observation.Explanation)
		if observation.Statement == "" && observation.Explanation != "" {
			observation.Statement = truncateCounselorText(observation.Explanation, maxObservationStatementCharacters)
		}
		if observation.Statement == "" {
			continue
		}
		if unsafeAuthorityClaim(observation.Statement) != "" || unsafeAdviceClaim(observation.Statement) != "" {
			continue
		}
		if observation.Explanation == "" || unsafeAuthorityClaim(observation.Explanation) != "" || unsafeAdviceClaim(observation.Explanation) != "" {
			observation.Explanation = "This retained observation is limited to the supporting evidence listed below."
		}
		observation.Statement = truncateCounselorText(observation.Statement, maxObservationStatementCharacters)
		observation.Explanation = truncateCounselorText(observation.Explanation, maxExplanationCharacters)
		observation.SupportingFactIDs = filterKnownFactIDs(normalizeFactIDs(observation.SupportingFactIDs), factByID)
		observation.ReportedValues = correctedReportedValues(observation.ReportedValues, observation.SupportingFactIDs, factByID)
		for _, value := range observation.ReportedValues {
			if !containsString(observation.SupportingFactIDs, value.FactID) {
				observation.SupportingFactIDs = append(observation.SupportingFactIDs, value.FactID)
			}
		}
		observation.Citations = acceptedCounselorCitations(observation.Citations, tools)
		observation.Limitations = sanitizeCounselorStrings(observation.Limitations, maxObservationLimitations)
		observation.Category = correctedObservationCategory(observation, factByID)
		if observation.Category == CategorySuppliedFact || observation.Category == CategoryDeterministicDerivedFact {
			observation.Citations = []CounselorCitation{}
			observation.SupportingFactIDs = limitDirectNumericSupport(observation.SupportingFactIDs, factByID)
			observation.ReportedValues = ensureReferencedNumericValues(observation.ReportedValues, observation.SupportingFactIDs, factByID)
		}
		if observation.Category == CategorySourcedExternalEvidence || observation.Category == CategoryUnavailableInformation {
			observation.ReportedValues = []ReportedValue{}
		}
		if observation.Category == CategoryGeneralInformation {
			observation.SupportingFactIDs = []string{}
			observation.ReportedValues = []ReportedValue{}
			observation.Citations = []CounselorCitation{}
		}
		if !validSeverity(observation.Severity) {
			observation.Severity = SeverityLimited
		}
		if !validConfidence(observation.Confidence) {
			observation.Confidence = ConfidenceLow
		}
		if observation.SupportingFactIDs == nil {
			observation.SupportingFactIDs = []string{}
		}
		if observation.ReportedValues == nil {
			observation.ReportedValues = []ReportedValue{}
		}
		if observation.Limitations == nil {
			observation.Limitations = []string{}
		}
		if observation.Citations == nil {
			observation.Citations = []CounselorCitation{}
		}
		cleaned = append(cleaned, observation)
	}

	if len(orderedFacts) > 0 {
		ordered := make([]CounselorObservation, 0, len(orderedFacts))
		for _, fact := range orderedFacts {
			selected := -1
			for index := range cleaned {
				if containsString(cleaned[index].SupportingFactIDs, fact.ID) {
					selected = index
					break
				}
			}
			if selected < 0 {
				ordered = append(ordered, unavailableCoverageObservation(fact))
				continue
			}
			observation := cleaned[selected]
			for _, other := range orderedFacts {
				if other.ID != fact.ID {
					observation.SupportingFactIDs = removeString(observation.SupportingFactIDs, other.ID)
				}
			}
			observation.Category = correctedObservationCategory(observation, factByID)
			ordered = append(ordered, observation)
			cleaned = append(cleaned[:selected], cleaned[selected+1:]...)
		}
		cleaned = ordered
	} else {
		for _, fact := range requiredFacts {
			covered := false
			for index := range cleaned {
				if containsString(cleaned[index].SupportingFactIDs, fact.ID) {
					covered = true
					continue
				}
				if cleaned[index].Category == CategorySourcedExternalEvidence || cleaned[index].Category == CategoryUnavailableInformation {
					cleaned[index].SupportingFactIDs = append(cleaned[index].SupportingFactIDs, fact.ID)
					covered = true
				}
			}
			if !covered {
				cleaned = append(cleaned, unavailableCoverageObservation(fact))
			}
		}
	}
	maximumObservations = effectiveMaxObservations(maximumObservations)
	if len(cleaned) > maximumObservations {
		cleaned = cleaned[:maximumObservations]
	}
	response.Observations = cleaned
	response.Limitations = sanitizeResponseLimitations(response.Limitations, factByID)
	if len(response.Observations) == 0 {
		return fallbackCounselorResponse(facts, maximumObservations)
	}
	return response
}

func fallbackCounselorResponse(facts []CounselorFact, maximumObservations int) CounselorResponse {
	response := CounselorResponse{
		SchemaVersion: counselorSchemaVersion,
		Summary:       "The model did not return a fully usable structured answer, so only a limited grounded result is shown.",
		Observations:  []CounselorObservation{},
		Limitations:   []CounselorLimitation{},
	}
	ordered := make([]CounselorFact, 0)
	required := make([]CounselorFact, 0)
	for _, fact := range facts {
		if fact.ObservationOrder > 0 {
			ordered = append(ordered, fact)
		} else if fact.RequiredObservation {
			required = append(required, fact)
		}
	}
	sort.SliceStable(ordered, func(left, right int) bool { return ordered[left].ObservationOrder < ordered[right].ObservationOrder })
	for _, fact := range append(ordered, required...) {
		response.Observations = append(response.Observations, unavailableCoverageObservation(fact))
	}
	if len(response.Observations) == 0 {
		var selected *CounselorFact
		for index := range facts {
			if facts[index].Available && facts[index].NumericValue != nil {
				selected = &facts[index]
				break
			}
			if selected == nil && facts[index].Available {
				selected = &facts[index]
			}
		}
		if selected != nil {
			observation := CounselorObservation{
				Category: ObservationCategory(selected.Kind), Statement: selected.Label + " was available to the analysis.",
				SupportingFactIDs: []string{selected.ID}, ReportedValues: []ReportedValue{}, Severity: SeverityLimited,
				Confidence: ConfidenceHigh, Limitations: []string{"The model response could not be retained in full."},
				Explanation: "This fallback preserves only an authoritative application fact.", Citations: []CounselorCitation{},
			}
			if selected.NumericValue != nil {
				observation.ReportedValues = []ReportedValue{{FactID: selected.ID, Value: *selected.NumericValue, Unit: selected.Unit}}
			}
			response.Observations = append(response.Observations, observation)
		} else if len(facts) > 0 {
			response.Observations = append(response.Observations, unavailableCoverageObservation(facts[0]))
		}
	}
	maximumObservations = effectiveMaxObservations(maximumObservations)
	if len(response.Observations) > maximumObservations {
		response.Observations = response.Observations[:maximumObservations]
	}
	return response
}

func unavailableCoverageObservation(fact CounselorFact) CounselorObservation {
	return CounselorObservation{
		Category: CategoryUnavailableInformation, Statement: "No usable grounded model observation was available for " + fact.Label + ".",
		SupportingFactIDs: []string{fact.ID}, ReportedValues: []ReportedValue{}, Severity: SeverityLimited, Confidence: ConfidenceLow,
		Limitations: []string{"The model response did not provide reliable coverage for this item."},
		Explanation: "The application retained the supplied identity but did not invent missing evidence.", Citations: []CounselorCitation{},
	}
}

func correctedObservationCategory(observation CounselorObservation, facts map[string]CounselorFact) ObservationCategory {
	if len(observation.Citations) > 0 {
		if !containsRequiredObservationFact(facts) || referencesRequiredObservationFact(factsForIDs(observation.SupportingFactIDs, facts)) {
			return CategorySourcedExternalEvidence
		}
		return CategoryInterpretation
	}
	referenced := factsForIDs(observation.SupportingFactIDs, facts)
	if referencesUnavailableFact(referenced) || (observation.Category == CategoryUnavailableInformation && len(observation.Limitations) > 0) {
		return CategoryUnavailableInformation
	}
	if allFactsHaveKind(referenced, CategorySuppliedFact) {
		return CategorySuppliedFact
	}
	if allFactsHaveKind(referenced, CategoryDeterministicDerivedFact) {
		return CategoryDeterministicDerivedFact
	}
	if len(referenced) > 0 {
		return CategoryInterpretation
	}
	return CategoryGeneralInformation
}

func factsForIDs(ids []string, facts map[string]CounselorFact) map[string]CounselorFact {
	result := make(map[string]CounselorFact, len(ids))
	for _, id := range ids {
		if fact, ok := facts[id]; ok {
			result[id] = fact
		}
	}
	return result
}

func filterKnownFactIDs(ids []string, facts map[string]CounselorFact) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := facts[id]; ok && !containsString(result, id) {
			result = append(result, id)
		}
	}
	if len(result) > maxSupportingFactsPerObservation {
		result = result[:maxSupportingFactsPerObservation]
	}
	return result
}

func limitDirectNumericSupport(ids []string, facts map[string]CounselorFact) []string {
	result := make([]string, 0, len(ids))
	numericCount := 0
	for _, id := range ids {
		fact := facts[id]
		if fact.Available && fact.NumericValue != nil {
			if numericCount == maxReportedValuesPerObservation {
				continue
			}
			numericCount++
		}
		result = append(result, id)
	}
	return result
}

func correctedReportedValues(values []ReportedValue, supporting []string, facts map[string]CounselorFact) []ReportedValue {
	result := make([]ReportedValue, 0, len(values))
	for _, value := range values {
		value.FactID = normalizeFactID(value.FactID)
		fact, ok := facts[value.FactID]
		if !ok || !fact.Available || fact.NumericValue == nil || containsReportedFact(result, value.FactID) {
			continue
		}
		result = append(result, ReportedValue{FactID: value.FactID, Value: *fact.NumericValue, Unit: fact.Unit})
	}
	if len(result) > maxReportedValuesPerObservation {
		result = result[:maxReportedValuesPerObservation]
	}
	return result
}

func ensureReferencedNumericValues(values []ReportedValue, supporting []string, facts map[string]CounselorFact) []ReportedValue {
	for _, factID := range supporting {
		fact, ok := facts[factID]
		if ok && fact.Available && fact.NumericValue != nil && !containsReportedFact(values, factID) && len(values) < maxReportedValuesPerObservation {
			values = append(values, ReportedValue{FactID: factID, Value: *fact.NumericValue, Unit: fact.Unit})
		}
	}
	return values
}

func containsReportedFact(values []ReportedValue, factID string) bool {
	for _, value := range values {
		if value.FactID == factID {
			return true
		}
	}
	return false
}

func acceptedCounselorCitations(citations []CounselorCitation, tools map[string]Tool) []CounselorCitation {
	result := make([]CounselorCitation, 0, len(citations))
	for _, citation := range citations {
		if !acceptedCitation(citation.URL, tools) {
			continue
		}
		citation.Title = sanitizeCounselorText(citation.Title)
		if citation.Title == "" || unsafeAuthorityClaim(citation.Title) != "" || unsafeAdviceClaim(citation.Title) != "" {
			citation.Title = "Accepted research source"
		}
		result = append(result, citation)
		if len(result) == maxCitationsPerObservation {
			break
		}
	}
	return result
}

func sanitizeCounselorText(value string) string {
	value = counselorURLPattern.ReplaceAllString(value, "source omitted")
	return strings.TrimSpace(strings.Join(strings.Fields(value), " "))
}

func sanitizeCounselorStrings(values []string, limit int) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = sanitizeCounselorText(value)
		if value == "" || unsafeAuthorityClaim(value) != "" || unsafeAdviceClaim(value) != "" {
			continue
		}
		result = append(result, value)
		if len(result) == limit {
			break
		}
	}
	return result
}

func sanitizeResponseLimitations(values []CounselorLimitation, facts map[string]CounselorFact) []CounselorLimitation {
	result := make([]CounselorLimitation, 0, len(values))
	for _, limitation := range values {
		statement := sanitizeCounselorText(limitation.Statement)
		if statement == "" || unsafeAuthorityClaim(statement) != "" || unsafeAdviceClaim(statement) != "" {
			continue
		}
		factIDs := make([]string, 0, len(limitation.FactIDs))
		for _, factID := range normalizeFactIDs(limitation.FactIDs) {
			if _, ok := facts[factID]; ok && !containsString(factIDs, factID) {
				factIDs = append(factIDs, factID)
			}
		}
		result = append(result, CounselorLimitation{Statement: statement, FactIDs: factIDs})
	}
	return result
}

func appendSalvageLimitation(response *CounselorResponse) {
	const statement = "Some model output was omitted or corrected because it did not match the available facts, citations, or response format."
	for _, limitation := range response.Limitations {
		if limitation.Statement == statement {
			return
		}
	}
	response.Limitations = append(response.Limitations, CounselorLimitation{Statement: statement, FactIDs: []string{}})
}

func compactCounselorLimitations(response *CounselorResponse) {
	if len(response.Limitations) <= maxCounselorLimitations {
		return
	}
	kept := append([]CounselorLimitation(nil), response.Limitations[:maxCounselorLimitations-1]...)
	factIDs := []string{}
	for _, limitation := range response.Limitations[maxCounselorLimitations-1:] {
		for _, factID := range limitation.FactIDs {
			if !containsString(factIDs, factID) {
				factIDs = append(factIDs, factID)
			}
		}
	}
	kept = append(kept, CounselorLimitation{Statement: "Additional supplied data limitations also apply.", FactIDs: factIDs})
	response.Limitations = kept
}

func truncateCounselorText(value string, maximum int) string {
	runes := []rune(strings.TrimSpace(value))
	if maximum <= 0 || len(runes) <= maximum {
		return string(runes)
	}
	if maximum == 1 {
		return "…"
	}
	return strings.TrimSpace(string(runes[:maximum-1])) + "…"
}

func removeString(values []string, unwanted string) []string {
	result := values[:0]
	for _, value := range values {
		if value != unwanted {
			result = append(result, value)
		}
	}
	return result
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
	if response.SchemaVersion != counselorSchemaVersion || strings.TrimSpace(response.Summary) == "" || utf8.RuneCountInString(response.Summary) > maxSummaryCharacters || len(response.Observations) == 0 || len(response.Observations) > maxCounselorObservations || len(response.Limitations) == 0 || len(response.Limitations) > maxCounselorLimitations {
		return nil, fmt.Errorf("stored counselor response is incomplete")
	}
	allText := []string{response.Summary}
	for index, observation := range response.Observations {
		if !validCategory(observation.Category) || !validSeverity(observation.Severity) || !validConfidence(observation.Confidence) || strings.TrimSpace(observation.Statement) == "" || utf8.RuneCountInString(observation.Statement) > maxObservationStatementCharacters || strings.TrimSpace(observation.Explanation) == "" || utf8.RuneCountInString(observation.Explanation) > maxExplanationCharacters || observation.SupportingFactIDs == nil || len(observation.SupportingFactIDs) > maxSupportingFactsPerObservation || observation.ReportedValues == nil || len(observation.ReportedValues) > maxReportedValuesPerObservation || observation.Limitations == nil || len(observation.Limitations) > maxObservationLimitations || observation.Citations == nil || len(observation.Citations) > maxCitationsPerObservation {
			return nil, fmt.Errorf("stored counselor observation %d is invalid", index)
		}
		for _, factID := range observation.SupportingFactIDs {
			if strings.TrimSpace(factID) == "" {
				return nil, fmt.Errorf("stored counselor observation %d has an empty fact reference", index)
			}
		}
		for _, value := range observation.ReportedValues {
			if strings.TrimSpace(value.FactID) == "" || strings.TrimSpace(value.Unit) == "" || math.IsNaN(value.Value) || math.IsInf(value.Value, 0) {
				return nil, fmt.Errorf("stored counselor observation %d has an invalid reported value", index)
			}
		}
		allText = append(allText, observation.Statement, observation.Explanation, strings.Join(observation.Limitations, " "))
		for _, citation := range observation.Citations {
			allText = append(allText, citation.Title)
			parsed, err := url.Parse(strings.TrimSpace(citation.URL))
			if strings.TrimSpace(citation.Title) == "" || err != nil || parsed.Scheme != "https" || parsed.Host == "" {
				return nil, fmt.Errorf("stored counselor observation %d has an invalid citation", index)
			}
		}
	}
	for index, limitation := range response.Limitations {
		if strings.TrimSpace(limitation.Statement) == "" {
			return nil, fmt.Errorf("stored counselor limitation %d is empty", index)
		}
		for _, factID := range limitation.FactIDs {
			if strings.TrimSpace(factID) == "" {
				return nil, fmt.Errorf("stored counselor limitation %d has an empty fact reference", index)
			}
		}
		allText = append(allText, limitation.Statement)
	}
	joined := strings.Join(allText, " ")
	if unsafeAuthorityClaim(joined) != "" || unsafeAdviceClaim(joined) != "" {
		return nil, fmt.Errorf("stored counselor response violates the read-only safety boundary")
	}
	return &response, nil
}

// normalizeCounselorResponse repairs representation-only mistakes and appends backend-required limitations.
func normalizeCounselorResponse(response *CounselorResponse, facts []CounselorFact) {
	if response == nil {
		return
	}
	factByID := make(map[string]CounselorFact, len(facts))
	for _, fact := range facts {
		factByID[fact.ID] = fact
	}
	for index := range response.Observations {
		observation := &response.Observations[index]
		observation.SupportingFactIDs = normalizeFactIDs(observation.SupportingFactIDs)
		for valueIndex := range observation.ReportedValues {
			value := &observation.ReportedValues[valueIndex]
			value.FactID = normalizeFactID(value.FactID)
			switch strings.ToLower(strings.TrimSpace(value.Unit)) {
			case "%", "percentage", "percentages":
				value.Unit = "percent"
			case "percentage point", "percentage points", "percentage_point":
				value.Unit = "percentage_points"
			}
		}
	}
	coveredLimitations := map[string]bool{}
	for index := range response.Limitations {
		factIDs := normalizeFactIDs(response.Limitations[index].FactIDs)
		response.Limitations[index].FactIDs = factIDs
		for _, factID := range response.Limitations[index].FactIDs {
			coveredLimitations[factID] = true
		}
	}
	missingByStatement := map[string][]string{}
	statementOrder := make([]string, 0)
	for _, fact := range facts {
		if fact.RequiredLimitation && !coveredLimitations[fact.ID] {
			statement := strings.TrimSpace(fact.Limitation)
			if statement == "" {
				statement = fact.Label + " is unavailable or incomplete."
			}
			if _, exists := missingByStatement[statement]; !exists {
				statementOrder = append(statementOrder, statement)
			}
			missingByStatement[statement] = append(missingByStatement[statement], fact.ID)
		}
	}
	for _, statement := range statementOrder {
		response.Limitations = append(response.Limitations, CounselorLimitation{Statement: statement, FactIDs: missingByStatement[statement]})
	}
	if len(response.Limitations) == 0 {
		response.Limitations = []CounselorLimitation{{Statement: "This explanation is limited to the application-supplied facts and their timestamps.", FactIDs: []string{}}}
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
func validateCounselorResponse(response CounselorResponse, facts []CounselorFact, tools map[string]Tool, maximumObservations int) []string {
	errors := []string{}
	maximumObservations = effectiveMaxObservations(maximumObservations)
	if response.SchemaVersion != counselorSchemaVersion {
		errors = append(errors, fmt.Sprintf("schema_version must be %d", counselorSchemaVersion))
	}
	if strings.TrimSpace(response.Summary) == "" {
		errors = append(errors, "summary is required")
	}
	if utf8.RuneCountInString(response.Summary) > maxSummaryCharacters {
		errors = append(errors, fmt.Sprintf("summary exceeds %d characters", maxSummaryCharacters))
	}
	if len(response.Observations) == 0 {
		errors = append(errors, "at least one observation is required")
	}
	if len(response.Observations) > maximumObservations {
		errors = append(errors, fmt.Sprintf("observations exceed the maximum of %d", maximumObservations))
	}
	if len(response.Limitations) == 0 {
		errors = append(errors, "at least one response limitation is required")
	}
	if len(response.Limitations) > maxCounselorLimitations {
		errors = append(errors, fmt.Sprintf("limitations exceed the maximum of %d", maxCounselorLimitations))
	}

	factByID := make(map[string]CounselorFact, len(facts))
	orderedFactIDs := make(map[int]string)
	maximumObservationOrder := 0
	for _, fact := range facts {
		factByID[fact.ID] = fact
		if fact.ObservationOrder > 0 {
			orderedFactIDs[fact.ObservationOrder] = fact.ID
			if fact.ObservationOrder > maximumObservationOrder {
				maximumObservationOrder = fact.ObservationOrder
			}
		}
	}
	orderedObservationFacts := make([]string, 0, maximumObservationOrder)
	for order := 1; order <= maximumObservationOrder; order++ {
		if factID := orderedFactIDs[order]; factID != "" {
			orderedObservationFacts = append(orderedObservationFacts, factID)
		}
	}
	coveredLimitations := map[string]bool{}
	coveredObservations := map[string]bool{}
	requiredObservationCounts := map[string]int{}
	requiredObservationSequence := make([]string, 0, len(orderedObservationFacts))
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
		requiredInObservation := make([]string, 0, 1)
		for _, factID := range observation.SupportingFactIDs {
			coveredObservations[factID] = true
			if fact, ok := factByID[factID]; ok && fact.ObservationOrder > 0 {
				requiredInObservation = append(requiredInObservation, factID)
				requiredObservationCounts[factID]++
			}
		}
		if len(requiredInObservation) > 1 {
			errors = append(errors, fmt.Sprintf("observation %d combines multiple required-observation facts", index))
		} else if len(requiredInObservation) == 1 {
			requiredObservationSequence = append(requiredObservationSequence, requiredInObservation[0])
		}
		for _, citation := range observation.Citations {
			allText = append(allText, citation.Title)
		}
		errors = append(errors, validateObservation(index, observation, factByID, tools)...)
	}
	for _, fact := range facts {
		if fact.RequiredObservation && !coveredObservations[fact.ID] {
			errors = append(errors, fmt.Sprintf("required observation coverage for fact %q is missing", fact.ID))
		} else if fact.ObservationOrder > 0 && requiredObservationCounts[fact.ID] > 1 {
			errors = append(errors, fmt.Sprintf("required observation fact %q is covered more than once", fact.ID))
		}
	}
	if len(requiredObservationSequence) == len(orderedObservationFacts) {
		for index := range orderedObservationFacts {
			if requiredObservationSequence[index] != orderedObservationFacts[index] {
				errors = append(errors, "required-observation facts are not covered in supplied order")
				break
			}
		}
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
	if utf8.RuneCountInString(observation.Explanation) > maxExplanationCharacters {
		errors = append(errors, fmt.Sprintf("observation %d explanation exceeds %d characters", index, maxExplanationCharacters))
	}
	if len(observation.SupportingFactIDs) > maxSupportingFactsPerObservation {
		errors = append(errors, fmt.Sprintf("observation %d has too many supporting facts", index))
	}
	if len(observation.ReportedValues) > maxReportedValuesPerObservation {
		errors = append(errors, fmt.Sprintf("observation %d has too many reported values", index))
	}
	if len(observation.Citations) > maxCitationsPerObservation {
		errors = append(errors, fmt.Sprintf("observation %d has too many citations", index))
	}
	if len(observation.Limitations) > maxObservationLimitations {
		errors = append(errors, fmt.Sprintf("observation %d has too many limitations", index))
	}
	if observation.SupportingFactIDs == nil || observation.ReportedValues == nil || observation.Limitations == nil || observation.Citations == nil {
		errors = append(errors, fmt.Sprintf("observation %d requires every array field; use empty arrays when needed", index))
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
	if observation.Category == CategorySourcedExternalEvidence && containsRequiredObservationFact(facts) && !referencesRequiredObservationFact(referenced) {
		errors = append(errors, fmt.Sprintf("observation %d does not identify the researched instrument", index))
	}
	if observation.Category == CategoryUnavailableInformation {
		if !referencesUnavailableFact(referenced) && len(observation.Limitations) == 0 {
			errors = append(errors, fmt.Sprintf("observation %d must explain the unavailable evidence", index))
		}
	}
	if observation.Category == CategorySuppliedFact && !allFactsHaveKind(referenced, CategorySuppliedFact) {
		errors = append(errors, fmt.Sprintf("observation %d labels non-supplied support as a supplied fact", index))
	}
	if observation.Category == CategoryDeterministicDerivedFact && !allFactsHaveKind(referenced, CategoryDeterministicDerivedFact) {
		errors = append(errors, fmt.Sprintf("observation %d labels non-deterministic support as a deterministic fact", index))
	}
	if observation.Category == CategoryGeneralInformation && (len(observation.SupportingFactIDs) > 0 || len(observation.ReportedValues) > 0 || len(observation.Citations) > 0) {
		errors = append(errors, fmt.Sprintf("observation %d labels portfolio facts or research as general information", index))
	}
	if (observation.Category == CategorySuppliedFact || observation.Category == CategoryDeterministicDerivedFact) && len(observation.Citations) > 0 {
		errors = append(errors, fmt.Sprintf("observation %d mixes an application fact category with external citations", index))
	}
	if (observation.Category == CategorySourcedExternalEvidence || observation.Category == CategoryUnavailableInformation) && len(observation.ReportedValues) > 0 {
		errors = append(errors, fmt.Sprintf("observation %d reports application numeric values under an external or unavailable category", index))
	}
	reportedFactIDs := make(map[string]bool, len(observation.ReportedValues))
	for _, value := range observation.ReportedValues {
		reportedFactIDs[value.FactID] = true
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
	if observation.Category == CategorySuppliedFact || observation.Category == CategoryDeterministicDerivedFact {
		for factID, fact := range referenced {
			if fact.Available && fact.NumericValue != nil && !reportedFactIDs[factID] {
				errors = append(errors, fmt.Sprintf("observation %d references numeric fact %q without reporting its value and unit", index, factID))
			}
		}
	}
	for _, citation := range observation.Citations {
		if strings.TrimSpace(citation.Title) == "" {
			errors = append(errors, fmt.Sprintf("observation %d has a citation without a title", index))
		}
		if !acceptedCitation(citation.URL, tools) {
			errors = append(errors, fmt.Sprintf("observation %d cites URL outside the accepted research boundary: %q", index, citation.URL))
		}
	}
	return errors
}

func effectiveMaxObservations(configured int) int {
	if configured > 0 && configured <= maxCounselorObservations {
		return configured
	}
	return maxCounselorObservations
}

// validateCounselorFacts rejects ambiguous or duplicate authoritative inputs before inference.
func validateCounselorFacts(facts []CounselorFact) error {
	if len(facts) == 0 {
		return fmt.Errorf("at least one authoritative fact is required")
	}
	seen := make(map[string]struct{}, len(facts))
	observationOrders := make(map[int]string)
	for index, fact := range facts {
		id := strings.TrimSpace(fact.ID)
		if id == "" || normalizeFactID(id) != id || strings.ContainsAny(id, " |") {
			return fmt.Errorf("fact %d has an invalid ID %q", index, fact.ID)
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("fact ID %q is duplicated", id)
		}
		seen[id] = struct{}{}
		kind := ObservationCategory(fact.Kind)
		if kind != CategorySuppliedFact && kind != CategoryDeterministicDerivedFact && kind != CategoryUnavailableInformation {
			return fmt.Errorf("fact %q has invalid kind %q", id, fact.Kind)
		}
		if strings.TrimSpace(fact.Label) == "" {
			return fmt.Errorf("fact %q has no label", id)
		}
		if fact.ObservationOrder < 0 || (fact.ObservationOrder > 0 && !fact.RequiredObservation) {
			return fmt.Errorf("fact %q has an invalid observation order", id)
		}
		if prior, exists := observationOrders[fact.ObservationOrder]; fact.ObservationOrder > 0 && exists {
			return fmt.Errorf("facts %q and %q have duplicate observation order %d", prior, id, fact.ObservationOrder)
		} else if fact.ObservationOrder > 0 {
			observationOrders[fact.ObservationOrder] = id
		}
		if fact.Available {
			if kind == CategoryUnavailableInformation {
				return fmt.Errorf("available fact %q cannot use unavailable_information", id)
			}
			hasNumber := fact.NumericValue != nil
			hasText := strings.TrimSpace(fact.TextValue) != ""
			if hasNumber == hasText {
				return fmt.Errorf("available fact %q must contain exactly one text or numeric value", id)
			}
			if hasNumber {
				if strings.TrimSpace(fact.Unit) == "" || math.IsNaN(*fact.NumericValue) || math.IsInf(*fact.NumericValue, 0) {
					return fmt.Errorf("numeric fact %q requires a finite value and unit", id)
				}
				if fact.AbsoluteTolerance < 0 || fact.RelativeTolerance < 0 || math.IsNaN(fact.AbsoluteTolerance) || math.IsNaN(fact.RelativeTolerance) || math.IsInf(fact.AbsoluteTolerance, 0) || math.IsInf(fact.RelativeTolerance, 0) {
					return fmt.Errorf("numeric fact %q has an invalid tolerance", id)
				}
			}
		} else {
			if kind != CategoryUnavailableInformation || fact.NumericValue != nil || strings.TrimSpace(fact.Limitation) == "" {
				return fmt.Errorf("unavailable fact %q requires unavailable_information, no numeric value, and a limitation", id)
			}
			if fact.RequiredObservation {
				return fmt.Errorf("unavailable fact %q cannot require observation coverage", id)
			}
		}
	}
	for order := 1; order <= len(observationOrders); order++ {
		if _, exists := observationOrders[order]; !exists {
			return fmt.Errorf("required observation orders must be contiguous from 1; order %d is missing", order)
		}
	}
	return nil
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

func containsRequiredObservationFact(facts map[string]CounselorFact) bool {
	for _, fact := range facts {
		if fact.RequiredObservation {
			return true
		}
	}
	return false
}

func referencesRequiredObservationFact(facts map[string]CounselorFact) bool {
	for _, fact := range facts {
		if fact.RequiredObservation {
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
