package proof

func EffectCounts(trace RunTrace) (escalations, budgetWarnings, runtimeErrors int) {
	for _, event := range trace.Events {
		switch event.Type {
		case EventEscalationTriggered:
			escalations++
		case EventBudgetWarning:
			budgetWarnings++
		case EventError:
			runtimeErrors++
		}
	}
	return escalations, budgetWarnings, runtimeErrors
}
