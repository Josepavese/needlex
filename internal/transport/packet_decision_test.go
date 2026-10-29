package transport

import (
	"encoding/json"
	"strings"
	"testing"

	coreservice "github.com/josepavese/needlex/internal/core/service"
	"github.com/josepavese/needlex/internal/intel"
)

func TestCompactDecisionPreservesAdvisoryScope(t *testing.T) {
	response := coreservice.ReadResponse{}
	ordinary, _ := json.Marshal(compactReadResponse(response))
	if strings.Contains(string(ordinary), "decision_observation") {
		t.Fatal("ordinary output gained unsolicited model work")
	}
	response.AgentContext.Decision = &intel.DecisionObservation{Assertion: "37 months", EvidenceIDs: []string{"selected"}, UnassessedEvidenceCount: 4, DecisionExecution: intel.DecisionExecution{Status: "observed", Calibration: "unqualified_in_domain", Answers: map[string]intel.DecisionAnswer{"support": {Choice: "supported"}}}}
	compact := compactReadResponse(response)
	if compact.Decision == nil || compact.Decision.Verdict != "supported" || compact.Decision.UnassessedEvidenceCount != 4 || compact.Decision.Calibration != "unqualified_in_domain" || compact.Decision.EvidenceIDs[0] != "selected" {
		t.Fatal("compact output lost the scope of an unqualified verdict")
	}
}
