package state

import (
	"testing"
	"time"

	"github.com/juanfont/headscale/hscontrol/routes"
	"github.com/juanfont/headscale/hscontrol/types"
	"github.com/juanfont/headscale/hscontrol/types/change"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectSetsLastSeen(t *testing.T) {
	id := types.NodeID(1)
	store := NewNodeStore(types.Nodes{{ID: id}}, func([]types.NodeView) map[types.NodeID][]types.NodeView {
		return nil
	})
	store.Start()
	t.Cleanup(store.Stop)

	s := &State{
		nodeStore:     store,
		primaryRoutes: routes.New(),
	}

	before := time.Now()
	changes, generation := s.Connect(id)
	after := time.Now()

	require.NotZero(t, generation)
	require.Len(t, changes, 1)

	node, ok := s.GetNodeByID(id)
	require.True(t, ok)
	require.True(t, node.IsOnline().Valid())
	assert.True(t, node.IsOnline().Get())
	require.True(t, node.LastSeen().Valid())

	lastSeen := node.LastSeen().Get()
	assert.False(t, lastSeen.Before(before))
	assert.False(t, lastSeen.After(after))
	assert.Equal(t, change.NodeOnline(id, lastSeen), changes[0])
}

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
