package v2

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/juanfont/headscale/hscontrol/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPolicyManagerConcurrentReads(t *testing.T) {
	users := types.Users{
		{Model: gorm.Model{ID: 1}, Name: "user1", Email: "user1@headscale.net"},
		{Model: gorm.Model{ID: 2}, Name: "user2", Email: "user2@headscale.net"},
		{Model: gorm.Model{ID: 3}, Name: "user3", Email: "user3@headscale.net"},
	}
	policy := `{"acls":[{"action":"accept","src":["autogroup:member"],"dst":["autogroup:self:*"]}]}`

	const nodeCount = 60
	nodes := make(types.Nodes, 0, nodeCount)
	for i := range nodeCount {
		n := node(
			fmt.Sprintf("node%d", i),
			fmt.Sprintf("100.64.0.%d", i+1),
			fmt.Sprintf("fd7a:115c:a1e0::%d", i+1),
			users[i%len(users)],
			nil,
		)
		n.ID = types.NodeID(i + 1)
		nodes = append(nodes, n)
	}

	pm, err := NewPolicyManager([]byte(policy), users, nodes.ViewSlice())
	require.NoError(t, err)

	const (
		readers    = 16
		iterations = 60
		reloads    = 30
	)
	var wg sync.WaitGroup

	for reader := range readers {
		wg.Go(func() {
			for i := range iterations {
				nv := nodes[(reader+i)%len(nodes)].View()
				_, err := pm.FilterForNode(nv)
				assert.NoError(t, err)
				_, err = pm.MatchersForNode(nv)
				assert.NoError(t, err)
				pm.Filter()
				if i%8 == 0 {
					assert.NotNil(t, pm.BuildPeerMap(nodes.ViewSlice()))
				}
			}
		})
	}

	wg.Go(func() {
		for range reloads {
			_, err := pm.SetNodes(nodes.ViewSlice())
			assert.NoError(t, err)
		}
	})
	wg.Wait()
}

func TestPolicyManagerBuildPeerMapUsesSharedReadLock(t *testing.T) {
	users := types.Users{{Model: gorm.Model{ID: 1}, Name: "user1"}}
	n := node("node1", "100.64.0.1", "fd7a:115c:a1e0::1", users[0], nil)
	n.ID = 1
	nodes := types.Nodes{n}

	pm, err := NewPolicyManager([]byte(`{}`), users, nodes.ViewSlice())
	require.NoError(t, err)

	pm.mu.RLock()
	done := make(chan struct{})
	go func() {
		pm.BuildPeerMap(nodes.ViewSlice())
		close(done)
	}()

	select {
	case <-done:
		pm.mu.RUnlock()
	case <-time.After(time.Second):
		pm.mu.RUnlock()
		t.Fatal("BuildPeerMap serialized behind another policy reader")
	}
}
