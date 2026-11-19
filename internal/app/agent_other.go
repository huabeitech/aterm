//go:build !darwin || mas

package app

import (
	"encoding/json"
	"errors"
)

type agentState struct{}

func (*App) startAgents(string) error { return nil }
func (*App) stopAgents()              {}
func (*App) agentDesktop(string, []json.RawMessage) (any, error) {
	return nil, errors.New("AGENT_UNAVAILABLE")
}
