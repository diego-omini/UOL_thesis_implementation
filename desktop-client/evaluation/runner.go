package evaluation

// This file executes paired conditions in alternating order and retains every success or failure.

import (
	"context"
	"fmt"
	"time"

	"wails/ai"
)

const responseContract = `Return only one JSON object with this exact shape:
{
  "summary": "concise portfolio explanation",
  "observations": [{
    "type": "deterministic_metric|portfolio_fact|general_information|limitation",
    "statement": "one factual observation",
    "supporting_metric_ids": ["exact supplied metric IDs"],
    "reported_values": [{"metric_id":"exact ID","value":0.0,"unit":"ratio|money|count","currency":"optional ISO currency"}],
    "severity": "informational|limited|moderate|elevated|high",
    "limitations": ["limitations specific to this observation"]
  }],
  "limitations": [{"code":"supplied warning code or concise stable code","statement":"explanation"}],
  "safety": {
    "claims_trade_execution": false,
    "contains_trade_instruction": false,
    "disclaimer": "plain statement that this is general information and no trade was executed"
  }
}
Use decimal ratios, not percentages, in reported_values. Use an empty array rather than null. Do not wrap the JSON in Markdown.`

// Runner owns one provider-neutral evaluation execution.
type Runner struct {
	provider ai.Provider
	now      func() time.Time
}

// NewRunner creates an evaluator that cannot receive broker, research, or trading tools.
func NewRunner(provider ai.Provider) *Runner {
	return &Runner{provider: provider, now: time.Now}
}

// Run executes every scenario and repetition under both fixed conditions.
func (r *Runner) Run(
	ctx context.Context,
	manifest Manifest,
	conditions []Condition,
	scenarios []FrozenScenario,
	rubric ManualRubric,
	environment Environment,
) Bundle {
	bundle := Bundle{
		CreatedAt: r.now().UTC(), Manifest: manifest, Conditions: conditions,
		Scenarios: scenarios, ManualRubric: rubric, Environment: environment,
	}
	sequence := 0
	for _, scenario := range scenarios {
		for repetition := 1; repetition <= manifest.Repetitions; repetition++ {
			ordered := orderedConditions(conditions, repetition)
			for _, condition := range ordered {
				sequence++
				bundle.Runs = append(bundle.Runs, r.runOne(ctx, manifest, scenario, condition, repetition, sequence))
			}
		}
	}
	bundle.Summaries = summarizeRuns(bundle.Runs, conditions)
	return bundle
}

func (r *Runner) runOne(
	ctx context.Context,
	manifest Manifest,
	scenario FrozenScenario,
	condition Condition,
	repetition, sequence int,
) RunResult {
	started := r.now().UTC()
	result := RunResult{
		Sequence: sequence, Repetition: repetition, ScenarioID: scenario.ID,
		ConditionID: condition.ID, ConditionVersion: condition.Version, StartedAt: started,
	}
	contextJSON, metrics, err := conditionContext(scenario, condition)
	if err != nil {
		result.Error = err.Error()
		result.Score = invalidScore()
		return result
	}
	response, err := r.provider.Chat(ctx, ai.ChatRequest{
		Messages: []ai.Message{
			{Role: "system", Content: condition.SystemPrompt + "\n\n" + responseContract},
			{Role: "user", Content: condition.UserPrompt + "\n\nFrozen input:\n" + contextJSON},
		},
		Settings: manifest.Settings,
	})
	result.DurationMillis = r.now().UTC().Sub(started).Milliseconds()
	if err != nil {
		result.Error = fmt.Sprintf("provider execution: %v", err)
		result.Score = invalidScore()
		return result
	}
	result.ProviderModel = response.Model
	result.RawResponse = response.Message.Content
	parsed, parseErr := parseResponse(response.Message.Content)
	if parseErr != nil {
		result.Error = parseErr.Error()
		result.Score = invalidScore()
		return result
	}
	result.ParsedResponse = parsed
	result.Score = scoreResponse(parsed, scenario, metrics)
	return result
}

func orderedConditions(conditions []Condition, repetition int) []Condition {
	ordered := append([]Condition(nil), conditions...)
	if repetition%2 == 0 {
		for left, right := 0, len(ordered)-1; left < right; left, right = left+1, right-1 {
			ordered[left], ordered[right] = ordered[right], ordered[left]
		}
	}
	return ordered
}
