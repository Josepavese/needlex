package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/josepavese/needlex/internal/config"
	coreservice "github.com/josepavese/needlex/internal/core/service"
	"github.com/josepavese/needlex/internal/intel"
)

func TestAcquisitionOptionCrossesBothPublicTransports(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		runner := Runner{
			storeRoot: t.TempDir(), loadConfig: func(string) (config.Config, error) { return config.Defaults(), nil },
			read: func(_ context.Context, _ config.Config, r coreservice.ReadRequest) (coreservice.ReadResponse, error) {
				if r.ObserveAcquisition != enabled || r.Objective != "Warranty" {
					t.Fatal("read lost explicit observation scope")
				}
				return fakeResponse(), nil
			},
			query: func(_ context.Context, _ config.Config, r coreservice.QueryRequest) (coreservice.QueryResponse, error) {
				if r.ObserveAcquisition != enabled || r.Goal != "Warranty" {
					t.Fatal("query lost explicit observation scope")
				}
				return fakeQueryResponse(r), nil
			},
		}
		for _, command := range []string{"read", "query"} {
			args := []string{"https://example.com", "--json"}
			if command == "read" {
				args = append(args, "--objective", "Warranty")
			} else {
				args = append(args, "--goal", "Warranty", "--discovery", "off")
			}
			if enabled {
				args = append(args, "--observe-acquisition")
			}
			var out, errOut bytes.Buffer
			var code int
			if command == "read" {
				code = runner.runRead(args, &out, &errOut)
			} else {
				code = runner.runQuery(args, &out, &errOut)
			}
			if code != 0 {
				t.Fatalf("%s CLI failed: %s", command, errOut.String())
			}
			params := map[string]any{"url": "https://example.com", "seed_url": "https://example.com", "objective": "Warranty", "goal": "Warranty", "discovery_mode": "off", "observe_acquisition": enabled}
			if _, err := runner.callMCPTool(mcpToolCall{Name: "web_" + command, Arguments: params}); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestCompactAcquisitionPreservesAdvisoryBoundary(t *testing.T) {
	response := fakeResponse()
	raw, _ := json.Marshal(compactReadResponse(response))
	if strings.Contains(string(raw), "acquisition_observation") {
		t.Fatal("ordinary packet gained unsolicited acquisition work")
	}
	response.AgentContext.Acquisition = &intel.AcquisitionObservation{DecisionExecution: intel.DecisionExecution{Status: "observed", Calibration: "unqualified_in_domain"}, Input: intel.AcquisitionInput{AllowedActions: []string{intel.AcquisitionServe, intel.AcquisitionRender}}, ProposedAction: intel.AcquisitionServe, BaselineAction: intel.AcquisitionRender, UnassessedEvidenceCount: 7}
	compact := compactReadResponse(response).Acquisition
	if compact == nil || !compact.ObservationOnly || compact.ProposedAction == compact.BaselineAction || compact.UnassessedEvidenceCount != 7 || compact.Calibration != "unqualified_in_domain" {
		t.Fatal("compact packet obscured the proposal scope or actual route")
	}
}
