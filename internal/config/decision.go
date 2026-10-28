package config

import (
	"fmt"
	"net"
	"net/url"
)

// DecisionConfig enables a local, advisory evaluation. It never controls retrieval.
type DecisionConfig struct {
	Mode              string `json:"mode,omitempty"`
	BaseURL           string `json:"base_url,omitempty"`
	Model             string `json:"model,omitempty"`
	Revision          string `json:"revision,omitempty"`
	TimeoutMS         int64  `json:"timeout_ms,omitempty"`
	MaxQuestionTokens int    `json:"max_question_tokens,omitempty"`
	MaxInputBytes     int    `json:"max_input_bytes,omitempty"`
}

func (c DecisionConfig) Validate() error {
	if c.Mode == "" || c.Mode == "off" {
		return nil
	}
	if c.Mode != "observe" {
		return fmt.Errorf("decision.mode must be off or observe")
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Scheme != "http" || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback() || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("decision.base_url must be a loopback HTTP URL")
	}
	if c.Model == "" || c.Revision == "" || c.TimeoutMS <= 0 || c.MaxInputBytes <= 0 || c.MaxQuestionTokens <= 0 {
		return fmt.Errorf("decision observation requires model, revision, positive timeout_ms max_input_bytes and max_question_tokens")
	}
	return nil
}
