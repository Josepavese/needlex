// decision_observer evaluates the advisory contract against independently labelled
// examples. It never treats the model's own answers as reference labels.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/intel"
)

type example struct {
	ID       string              `json:"id"`
	Input    intel.DecisionInput `json:"input"`
	Expected map[string]string   `json:"expected"`
}

func main() {
	configPath := flag.String("config", "", "Needle-X config with local observation enabled")
	casesPath := flag.String("cases", "", "independently labelled JSON corpus")
	repeats := flag.Int("repeats", 3, "paired observations per example")
	flag.Parse()
	cfg, err := config.Load(*configPath)
	if err != nil {
		panic(err)
	}
	observer := intel.NewDecisionObserver(cfg.Decision, nil)
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
	encoder := json.NewEncoder(os.Stdout)
	for repeat := 0; repeat < *repeats; repeat++ {
		for _, sample := range cases {
			started := time.Now()
			result, err := observer.Observe(context.Background(), sample.Input)
			correct := 0
			abstentions := 0
			for id, expected := range sample.Expected {
				answer := result.Answers[id]
				if answer.Choice == expected {
					correct++
				}
				if answer.Choice == "unknown" {
					abstentions++
				}
			}
			reason := ""
			if err != nil {
				reason = err.Error()
			}
			if err := encoder.Encode(map[string]any{"case": sample.ID, "repeat": repeat, "expected": sample.Expected, "correct": correct, "total": len(sample.Expected), "abstentions": abstentions, "wall_ms": time.Since(started).Milliseconds(), "observation": result, "error": reason}); err != nil {
				panic(fmt.Errorf("write observation: %w", err))
			}
		}
	}
}
