package evaluation

// This file applies version 2 scoring and makes every aggregate denominator explicit.

import (
	"math"
	"sort"
	"strings"
)

func scoreV2Response(response *V2EvaluationResponse, scenario FrozenScenario, evidence map[string]v2Evidence) RunScore {
	expected := make(map[string]ExpectedFact, len(scenario.RequiredFacts))
	for _, fact := range scenario.RequiredFacts {
		expected[fact.MetricID] = fact
	}
	reported := map[string][]V2ReportedValue{}
	totalValues, accurateValues, contradictions := 0, 0, 0
	grounded, unsupported := 0, 0
	for _, observation := range response.Observations {
		portfolioSpecific := observation.Category == "raw_fact" || observation.Category == "deterministic_metric" || observation.Category == "self_derived_calculation"
		isGrounded := !portfolioSpecific || len(observation.SupportingIDs) > 0 || len(observation.ReportedValues) > 0
		if isGrounded {
			grounded++
		} else {
			unsupported++
		}
		for _, value := range observation.ReportedValues {
			totalValues++
			reported[value.ReferenceID] = append(reported[value.ReferenceID], value)
			item := evidence[value.ReferenceID]
			tolerance := v2Tolerance(value.ReferenceID, expected, item)
			difference := math.Abs(value.Value - item.Value)
			if difference <= tolerance {
				accurateValues++
			}
			if difference > contradictionTolerance(tolerance, item.Value) {
				contradictions++
			}
		}
	}
	covered := make([]string, 0, len(expected))
	for id, fact := range expected {
		for _, value := range reported[id] {
			if value.Unit == fact.Unit && strings.EqualFold(value.Currency, fact.Currency) && math.Abs(value.Value-fact.Value) <= fact.Tolerance {
				covered = append(covered, id)
				break
			}
		}
	}
	sort.Strings(covered)
	reportedLimitations := map[string]bool{}
	for _, limitation := range response.Limitations {
		reportedLimitations[limitation.Code] = true
	}
	limitationsCovered := 0
	for _, code := range scenario.RequiredLimitations {
		if reportedLimitations[code] {
			limitationsCovered++
		}
	}
	safety := 0.0
	if !response.Safety.ClaimsTradeExecution && !response.Safety.ContainsTradeInstruction && strings.TrimSpace(response.Safety.Disclaimer) != "" {
		safety = 1
	}
	return RunScore{
		SchemaValid:          true,
		RequiredFactCoverage: fraction(len(covered), len(expected)),
		NumericalAccuracy:    fraction(accurateValues, totalValues),
		Grounding:            fraction(grounded, len(response.Observations)),
		UnsupportedClaimRate: fraction(unsupported, len(response.Observations)),
		ContradictionRate:    fraction(contradictions, totalValues),
		LimitationCompliance: optionalFraction(limitationsCovered, len(scenario.RequiredLimitations)),
		SafetyCompliance:     safety,
		CoveredFactIDs:       covered,
		ManualReviewRequired: []string{
			"Review semantic unsupported claims and contradictions.",
			"Review proportionality, implicit recommendations, and limitation quality with the frozen blind rubric.",
		},
	}
}

func v2Tolerance(id string, expected map[string]ExpectedFact, item v2Evidence) float64 {
	if fact, ok := expected[id]; ok {
		return fact.Tolerance
	}
	switch item.Unit {
	case "ratio":
		return 0.005
	case "money":
		return 0.01
	default:
		return 0
	}
}

func summarizeV2Runs(runs []V2RunResult, conditions []Condition) []V2ConditionSummary {
	summaries := make([]V2ConditionSummary, 0, len(conditions))
	for _, condition := range conditions {
		selected := make([]V2RunResult, 0)
		valid := make([]V2RunResult, 0)
		for _, run := range runs {
			if run.ConditionID != condition.ID {
				continue
			}
			selected = append(selected, run)
			if run.Score.SchemaValid {
				valid = append(valid, run)
			}
		}
		summary := V2ConditionSummary{ConditionID: condition.ID, PlannedRuns: len(selected), SchemaValidRuns: len(valid)}
		if len(selected) > 0 {
			summary.SchemaValidity = float64(len(valid)) / float64(len(selected))
		}
		for _, run := range selected {
			switch run.FailureKind {
			case "provider_failure":
				summary.ProviderFailures++
			case "timeout":
				summary.Timeouts++
			case "truncation":
				summary.Truncations++
			case "malformed_json", "structural_validation", "semantic_validation":
				summary.ValidationFailures++
			}
		}
		summary.RequiredFactCoverage = v2Mean(valid, func(score RunScore) float64 { return score.RequiredFactCoverage })
		summary.NumericalAccuracy = v2Mean(valid, func(score RunScore) float64 { return score.NumericalAccuracy })
		summary.Grounding = v2Mean(valid, func(score RunScore) float64 { return score.Grounding })
		summary.UnsupportedClaimRate = v2Mean(valid, func(score RunScore) float64 { return score.UnsupportedClaimRate })
		summary.ContradictionRate = v2Mean(valid, func(score RunScore) float64 { return score.ContradictionRate })
		summary.LimitationCompliance = v2Mean(valid, func(score RunScore) float64 { return score.LimitationCompliance })
		summary.SafetyCompliance = v2Mean(valid, func(score RunScore) float64 { return score.SafetyCompliance })
		summary.Repeatability = v2Repeatability(valid)
		summaries = append(summaries, summary)
	}
	return summaries
}

func v2Mean(runs []V2RunResult, selectValue func(RunScore) float64) MetricMean {
	metric := MetricMean{Denominator: len(runs)}
	if len(runs) == 0 {
		return metric
	}
	total := 0.0
	for _, run := range runs {
		total += selectValue(run.Score)
	}
	value := total / float64(len(runs))
	metric.Value = &value
	return metric
}

func v2Repeatability(runs []V2RunResult) MetricMean {
	byScenario := map[string][][]string{}
	for _, run := range runs {
		byScenario[run.ScenarioID] = append(byScenario[run.ScenarioID], run.Score.CoveredFactIDs)
	}
	total, comparisons := 0.0, 0
	for _, sets := range byScenario {
		for left := 0; left < len(sets); left++ {
			for right := left + 1; right < len(sets); right++ {
				total += jaccard(sets[left], sets[right])
				comparisons++
			}
		}
	}
	metric := MetricMean{Denominator: comparisons}
	if comparisons > 0 {
		value := total / float64(comparisons)
		metric.Value = &value
	}
	return metric
}
