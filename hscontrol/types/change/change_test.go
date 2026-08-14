package change

import (
	"testing"
	"time"

	"github.com/juanfont/headscale/hscontrol/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNodeOnlineOfflineIncludeLastSeen(t *testing.T) {
	t.Parallel()

	lastSeen := time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC)
	id := types.NodeID(1)

	tests := []struct {
		name       string
		change     ChangeSet
		wantChange Change
	}{
		{name: "online", change: NodeOnline(id, lastSeen), wantChange: NodeCameOnline},
		{name: "offline", change: NodeOffline(id, lastSeen), wantChange: NodeWentOffline},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.wantChange, test.change.Change)
			assert.Equal(t, id, test.change.NodeID)
			require.NotNil(t, test.change.LastSeen)
			assert.Equal(t, lastSeen, *test.change.LastSeen)
		})
	}
}
