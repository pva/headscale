package state

import (
	"net/netip"
	"testing"
	"time"

	"github.com/juanfont/headscale/hscontrol/routes"
	"github.com/juanfont/headscale/hscontrol/types"
	"github.com/juanfont/headscale/hscontrol/types/change"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tailscale.com/tailcfg"
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

func TestStaleDisconnectPreservesUniqueAndOverlappingPrimaryRoutes(t *testing.T) {
	proxyID := types.NodeID(100)
	routerID := types.NodeID(423)
	shared := netip.MustParsePrefix("185.76.151.0/24")
	unique := []netip.Prefix{
		netip.MustParsePrefix("172.16.144.0/23"),
		netip.MustParsePrefix("172.16.146.0/23"),
		netip.MustParsePrefix("10.69.1.0/24"),
	}
	exitV4 := netip.MustParsePrefix("0.0.0.0/0")
	exitV6 := netip.MustParsePrefix("::/0")
	routerRoutes := append([]netip.Prefix{shared}, unique...)
	routerRoutes = append(routerRoutes, exitV4, exitV6)

	store := NewNodeStore(types.Nodes{
		{
			ID:             proxyID,
			Hostinfo:       &tailcfg.Hostinfo{RoutableIPs: []netip.Prefix{shared}},
			ApprovedRoutes: []netip.Prefix{shared},
		},
		{
			ID:             routerID,
			Hostinfo:       &tailcfg.Hostinfo{RoutableIPs: routerRoutes},
			ApprovedRoutes: routerRoutes,
		},
	}, func([]types.NodeView) map[types.NodeID][]types.NodeView {
		return nil
	})
	store.Start()
	t.Cleanup(store.Stop)

	s := &State{
		nodeStore:     store,
		primaryRoutes: routes.New(),
	}

	_, _ = s.Connect(proxyID)
	_, staleGeneration := s.Connect(routerID)
	_, currentGeneration := s.Connect(routerID)
	require.Greater(t, currentGeneration, staleGeneration)

	changes, err := s.Disconnect(routerID, staleGeneration)
	require.NoError(t, err)
	assert.Nil(t, changes)

	node, ok := s.GetNodeByID(routerID)
	require.True(t, ok)
	require.True(t, node.IsOnline().Valid())
	assert.True(t, node.IsOnline().Get())

	assert.ElementsMatch(t, unique, s.GetNodePrimaryRoutes(routerID))
	assert.Equal(t, []netip.Prefix{shared}, s.GetNodePrimaryRoutes(proxyID))
}
