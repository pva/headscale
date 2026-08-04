package mapper

import (
	"testing"
	"time"

	"github.com/juanfont/headscale/hscontrol/types"
	"github.com/juanfont/headscale/hscontrol/types/change"
)

func TestProcessBatchedChangesBundlesChangesPerNode(t *testing.T) {
	b := NewBatcher(time.Hour, 1, nil)
	defer b.Close()

	b.pendingChanges.Store(types.NodeID(1), []change.ChangeSet{
		change.DERPSet,
		change.PolicySet,
		change.FullSet,
	})
	b.pendingChanges.Store(types.NodeID(2), []change.ChangeSet{change.DERPSet})

	b.processBatchedChanges()

	if got := len(b.workCh); got != 2 {
		t.Fatalf("work items = %d, want one per node (2)", got)
	}

	changesByNode := make(map[types.NodeID]int)
	for range 2 {
		w := <-b.workCh
		changesByNode[w.nodeID] = len(w.changes)
	}

	if got := changesByNode[1]; got != 3 {
		t.Errorf("node 1 changes = %d, want 3", got)
	}
	if got := changesByNode[2]; got != 1 {
		t.Errorf("node 2 changes = %d, want 1", got)
	}
}
