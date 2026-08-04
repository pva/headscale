package state

import (
	"testing"
	"time"

	"github.com/juanfont/headscale/hscontrol/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistrationCacheBoundedLRU(t *testing.T) {
	const maxEntries = 4

	cache := newRegistrationCache(time.Hour, maxEntries)
	entries := make([]types.RegisterNode, 0, maxEntries+1)
	ids := make([]types.RegistrationID, 0, maxEntries+1)

	for range maxEntries + 1 {
		id, err := types.NewRegistrationID()
		require.NoError(t, err)

		entry := types.NewRegisterNode(types.Node{})
		cache.Add(id, entry)
		ids = append(ids, id)
		entries = append(entries, entry)
	}

	assert.Equal(t, maxEntries, cache.Len())

	_, ok := cache.Get(ids[0])
	assert.False(t, ok, "oldest entry must be evicted when the cache is full")

	select {
	case node, open := <-entries[0].Registered:
		assert.Nil(t, node)
		assert.False(t, open, "eviction must close the registration channel")
	case <-time.After(time.Second):
		t.Fatal("eviction did not wake the pending registration")
	}

	for i := 1; i <= maxEntries; i++ {
		_, ok := cache.Get(ids[i])
		assert.True(t, ok, "non-evicted entry %d should remain cached", i)
	}
}
