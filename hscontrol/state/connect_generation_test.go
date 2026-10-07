package state

import (
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/juanfont/headscale/hscontrol/routes"
	"github.com/juanfont/headscale/hscontrol/types"
	"github.com/juanfont/headscale/hscontrol/types/change"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

// TestInFlightDisconnectDoesNotClearNewerSessionRoutes covers an old poll
// session that passes the generation check in Disconnect and then stalls
// (here: in the database write) while a new session connects. The old
// Disconnect must not clear the routes of the new session when it resumes.
func TestInFlightDisconnectDoesNotClearNewerSessionRoutes(t *testing.T) {
	prefixV4 := netip.MustParsePrefix("100.64.0.0/10")
	prefixV6 := netip.MustParsePrefix("fd7a:115c:a1e0::/48")

	s, err := NewState(&types.Config{
		Database: types.DatabaseConfig{
			Type:   types.DatabaseSqlite,
			Sqlite: types.SqliteConfig{Path: t.TempDir() + "/headscale_test.db"},
		},
		PrefixV4:     &prefixV4,
		PrefixV6:     &prefixV6,
		IPAllocation: types.IPAllocationStrategySequential,
		Policy:       types.PolicyConfig{Mode: types.PolicyModeDB},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	route := netip.MustParsePrefix("185.76.151.0/24")
	node := *s.CreateRegisteredNodeForTest(s.CreateUserForTest("user"), "proxies")
	node.Hostinfo = &tailcfg.Hostinfo{RoutableIPs: []netip.Prefix{route}}
	node.ApprovedRoutes = []netip.Prefix{route}
	s.nodeStore.PutNode(node)

	var blockDB atomic.Bool
	dbEntered := make(chan struct{})
	dbRelease := make(chan struct{})
	require.NoError(t, s.db.DB.Callback().Update().Before("gorm:update").
		Register("test:block_update", func(*gorm.DB) {
			if blockDB.CompareAndSwap(true, false) {
				close(dbEntered)
				<-dbRelease
			}
		}))

	_, oldGen := s.Connect(node.ID)
	require.Equal(t, []netip.Prefix{route}, s.GetNodePrimaryRoutes(node.ID))

	blockDB.Store(true)
	disconnectDone := make(chan struct{})
	go func() {
		defer close(disconnectDone)
		_, err := s.Disconnect(node.ID, oldGen)
		assert.NoError(t, err)
	}()

	select {
	case <-dbEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("old Disconnect did not reach the database write")
	}

	connectDone := make(chan struct{})
	go func() {
		defer close(connectDone)
		_, _ = s.Connect(node.ID)
	}()

	// Give the new Connect a chance to run to completion before the old
	// Disconnect resumes. With Connect and Disconnect serialised it cannot
	// finish here and the wait times out.
	select {
	case <-connectDone:
	case <-time.After(2 * batchTimeout):
	}

	close(dbRelease)

	for _, done := range []chan struct{}{disconnectDone, connectDone} {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Connect/Disconnect did not finish")
		}
	}

	nv, ok := s.GetNodeByID(node.ID)
	require.True(t, ok)
	require.True(t, nv.IsOnline().Valid())
	assert.True(t, nv.IsOnline().Get())
	assert.Equal(t, []netip.Prefix{route}, s.GetNodePrimaryRoutes(node.ID))
}
