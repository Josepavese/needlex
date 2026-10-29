// decision_observer evaluates the advisory contract against independently labelled
// examples. It never treats the model's own answers as reference labels.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/intel"
)

type example struct {
	Partition string              `json:"partition"`
	ID        string              `json:"id"`
	Input     intel.DecisionInput `json:"input"`
	Expected  map[string]string   `json:"expected"`
}

func main() {
	configPath := flag.String("config", "", "Needle-X config with local observation enabled")
	casesPath := flag.String("cases", "", "independently labelled JSON corpus")
	repeats := flag.Int("repeats", 1, "observations per example")
	partition := flag.String("partition", "", "evaluate only this frozen source-family partition")
	encoding := flag.String("encoding", "plain", "state encoding ablation: plain or json")
	referenceOnly := flag.Bool("reference-only", false, "collect dense reference without repeating model inference")
	withReference := flag.Bool("with-semantic-reference", false, "record dense similarity as a separate experimental reference")
	flag.Parse()
	cfg, err := config.Load(*configPath)
	if err != nil {
		panic(err)
	}
	semantic := intel.NewSemanticAligner(cfg, nil)
	observer := intel.NewDecisionObserver(cfg.Decision, nil, semantic)
	if observer == nil {
		panic("decision observation is disabled")
	}
	data, err := os.ReadFile(*casesPath)
	if err != nil {
		panic(err)
	}
	var cases []example
	if err := json.Unmarshal(data, &cases); err != nil {
		panic(err)
	}
	selected := []example{}
	for _, sample := range cases {
		if *partition != "" && sample.Partition != *partition {
			continue
		}
		if len(sample.Input.Evidence) != 1 {
			panic("qualification cases require exactly one source unit")
		}
		if strings.TrimSpace(sample.Input.Assertion) == "" {
			panic("claim-support corpus requires an explicit assertion")
		}
		for key := range sample.Expected {
			if key != "support" {
				panic("unknown claim-support judgment: " + key)
			}
		}
		selected = append(selected, sample)
	}
	if len(selected) == 0 {
		panic("no cases in requested partition")
	}
	if *encoding != "plain" && *encoding != "json" {
		panic("encoding must be plain or json")
	}
	encoder := json.NewEncoder(os.Stdout)
	for repeat := 0; repeat < *repeats; repeat++ {
		for _, sample := range selected {
			var reference *float64
			referenceReason := ""
			if *withReference || *referenceOnly {
				candidates := []intel.SemanticCandidate{}
				for _, evidence := range sample.Input.Evidence {
					candidates = append(candidates, intel.SemanticCandidate{ID: evidence.ID, Heading: evidence.Heading, Text: evidence.Text})
				}
				scores, scoreErr := semantic.Score(context.Background(), sample.Input.Assertion, candidates)
				if scoreErr != nil {
					referenceReason = scoreErr.Error()
				} else if len(scores) == 1 && scores[0].ID == sample.Input.Evidence[0].ID && !math.IsNaN(scores[0].Similarity) && !math.IsInf(scores[0].Similarity, 0) {
					value := scores[0].Similarity
					reference = &value
				} else {
					referenceReason = "dense reference unavailable"
				}
			}
			if *referenceOnly {
				if err := encoder.Encode(map[string]any{"case": sample.ID, "partition": sample.Partition, "reference_similarity": reference, "reference_error": referenceReason}); err != nil {
					panic(err)
				}
				continue
			}
			if *encoding == "json" {
				envelope := []intel.DecisionEvidence{}
				for _, evidence := range sample.Input.Evidence {
					envelope = append(envelope, intel.DecisionEvidence{ID: evidence.ID, Text: strings.Join(append(append([]string{}, evidence.Heading...), evidence.Text), "\n")})
				}
				data, _ := json.Marshal(map[string]any{"evidence": envelope})
				sample.Input.Evidence = []intel.DecisionEvidence{{ID: sample.Input.Evidence[0].ID, Text: string(data)}}
			}
			started := time.Now()
			result, err := observer.Observe(context.Background(), sample.Input)
			correct := 0
			abstentions := 0
			for id, expected := range sample.Expected {
				answer := result.Answers[id]
				if answer.Choice == expected {
					correct++
				}
				if answer.Choice == "not_established" {
					abstentions++
				}
			}
			reason := ""
			if err != nil {
				reason = err.Error()
			}
			if err := encoder.Encode(map[string]any{"case": sample.ID, "partition": sample.Partition, "encoding": *encoding, "reference_similarity": reference, "reference_error": referenceReason, "repeat": repeat, "expected": sample.Expected, "correct": correct, "total": len(sample.Expected), "abstentions": abstentions, "wall_ms": time.Since(started).Milliseconds(), "observation": result, "error": reason}); err != nil {
				panic(fmt.Errorf("write observation: %w", err))
			}
		}
	}
}
