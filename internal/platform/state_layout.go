package platform

import (
	"path/filepath"
	"strings"
)

const DefaultDiscoveryDBRelativePath = "discovery/discovery.db"

type StateLayout struct {
	Root                string `json:"root"`
	AnalyticsDir        string `json:"analytics_dir"`
	AnalyticsDB         string `json:"analytics_db"`
	CandidatesDir       string `json:"candidates_dir"`
	DiscoveryDir        string `json:"discovery_dir"`
	DiscoveryDB         string `json:"discovery_db"`
	DomainGraphDir      string `json:"domain_graph_dir"`
	FingerprintGraphDir string `json:"fingerprint_graph_dir"`
	FingerprintsDir     string `json:"fingerprints_dir"`
	GenomeDir           string `json:"genome_dir"`
	LogsDir             string `json:"logs_dir"`
	RuntimeLog          string `json:"runtime_log"`
	ProofsDir           string `json:"proofs_dir"`
	TracesDir           string `json:"traces_dir"`
}

func NewStateLayout(root string) StateLayout {
	cleanRoot := strings.TrimSpace(root)
	if cleanRoot == "" {
		cleanRoot = DefaultStateRoot()
	}
	return StateLayout{
		Root:                cleanRoot,
		AnalyticsDir:        filepath.Join(cleanRoot, "analytics"),
		AnalyticsDB:         filepath.Join(cleanRoot, "analytics", "analytics.db"),
		CandidatesDir:       filepath.Join(cleanRoot, "candidates"),
		DiscoveryDir:        filepath.Join(cleanRoot, "discovery"),
		DiscoveryDB:         filepath.Join(cleanRoot, DefaultDiscoveryDBRelativePath),
		DomainGraphDir:      filepath.Join(cleanRoot, "domain_graph"),
		FingerprintGraphDir: filepath.Join(cleanRoot, "fingerprint_graph"),
		FingerprintsDir:     filepath.Join(cleanRoot, "fingerprints"),
		GenomeDir:           filepath.Join(cleanRoot, "genome"),
		LogsDir:             filepath.Join(cleanRoot, "logs"),
		RuntimeLog:          filepath.Join(cleanRoot, "logs", "needlex.jsonl"),
		ProofsDir:           filepath.Join(cleanRoot, "proofs"),
		TracesDir:           filepath.Join(cleanRoot, "traces"),
	}
}

func (l StateLayout) Paths() map[string]string {
	return map[string]string{
		"analytics":         l.AnalyticsDir,
		"candidates":        l.CandidatesDir,
		"discovery":         l.DiscoveryDir,
		"domain_graph":      l.DomainGraphDir,
		"fingerprint_graph": l.FingerprintGraphDir,
		"fingerprints":      l.FingerprintsDir,
		"genome":            l.GenomeDir,
		"logs":              l.LogsDir,
		"proofs":            l.ProofsDir,
		"traces":            l.TracesDir,
	}
}
