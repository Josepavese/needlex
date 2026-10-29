package intel

import (
	"maps"
	"sync"
)

// A memo lives with its observer, is bounded, and retains private answer copies.
// Config replacement/process shutdown retires it. Source attribution
// and unassessed evidence are rebuilt for each call, never borrowed from cache.
const decisionMemoEntries = 128

type decisionMemo struct {
	mu     sync.Mutex
	values map[[32]byte]DecisionAnswer
	order  [][32]byte
}

func (m *decisionMemo) get(key [32]byte) (DecisionAnswer, bool) {
	m.mu.Lock()
	answer, ok := m.values[key]
	m.mu.Unlock()
	answer.Probabilities = maps.Clone(answer.Probabilities)
	return answer, ok
}

func (m *decisionMemo) set(key [32]byte, answer DecisionAnswer) {
	answer.Probabilities = maps.Clone(answer.Probabilities)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.values == nil {
		m.values = map[[32]byte]DecisionAnswer{}
	}
	if _, exists := m.values[key]; exists {
		return
	}
	if len(m.order) == decisionMemoEntries {
		delete(m.values, m.order[0])
		m.order = m.order[1:]
	}
	m.values[key] = answer
	m.order = append(m.order, key)
}
