package state

import (
	"testing"

	"github.com/juanfont/headscale/hscontrol/types"
)

func TestDisconnectRejectsStaleConnectGeneration(t *testing.T) {
	s := &State{}
	id := types.NodeID(1)

	stale := s.nextConnectGen(id)
	current := s.nextConnectGen(id)
	if current <= stale {
		t.Fatalf("connect generation did not increase: stale=%d current=%d", stale, current)
	}

	changes, err := s.Disconnect(id, stale)
	if err != nil {
		t.Fatalf("stale Disconnect returned error: %v", err)
	}
	if changes != nil {
		t.Fatalf("stale Disconnect returned changes: %v", changes)
	}
}
