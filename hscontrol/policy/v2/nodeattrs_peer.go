package v2

import (
	"github.com/juanfont/headscale/hscontrol/types"
	"tailscale.com/tailcfg"
)

// PeerCapMap returns the subset of peerSelfCaps that the Tailscale client
// reads from the peer view, provided that the peer satisfies the capability's
// emission conditions. Most peers have no peer-view capabilities.
func PeerCapMap(peer types.NodeView, peerSelfCaps tailcfg.NodeCapMap) tailcfg.NodeCapMap {
	if len(peerSelfCaps) == 0 {
		return nil
	}

	var out tailcfg.NodeCapMap

	// Only suggest an exit node after its advertised exit route has been
	// approved. This prevents the UI suggestion from trusting an unapproved
	// route merely because policy assigned the capability to the node.
	if peer.IsExitNode() {
		if value, ok := peerSelfCaps[tailcfg.NodeAttrSuggestExitNode]; ok {
			out = tailcfg.NodeCapMap{
				tailcfg.NodeAttrSuggestExitNode: value,
			}
		}
	}

	return out
}
