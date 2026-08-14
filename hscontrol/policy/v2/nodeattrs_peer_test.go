package v2

import (
	"net/netip"
	"testing"

	"github.com/juanfont/headscale/hscontrol/types"
	"github.com/stretchr/testify/assert"
	"tailscale.com/tailcfg"
)

func TestPeerCapMapSuggestExitNode(t *testing.T) {
	t.Parallel()

	exitRoute := netip.MustParsePrefix("0.0.0.0/0")
	suggestExitNode := tailcfg.NodeCapMap{
		tailcfg.NodeAttrSuggestExitNode:     nil,
		tailcfg.NodeAttrRandomizeClientPort: nil,
	}

	tests := []struct {
		name           string
		announced      []netip.Prefix
		approved       []netip.Prefix
		selfCaps       tailcfg.NodeCapMap
		wantPeerCapMap tailcfg.NodeCapMap
	}{
		{
			name:     "no policy capabilities",
			selfCaps: nil,
		},
		{
			name:     "exit route not announced",
			approved: []netip.Prefix{exitRoute},
			selfCaps: suggestExitNode,
		},
		{
			name:      "exit route not approved",
			announced: []netip.Prefix{exitRoute},
			selfCaps:  suggestExitNode,
		},
		{
			name:      "suggest capability absent",
			announced: []netip.Prefix{exitRoute},
			approved:  []netip.Prefix{exitRoute},
			selfCaps: tailcfg.NodeCapMap{
				tailcfg.NodeAttrRandomizeClientPort: nil,
			},
		},
		{
			name:      "approved exit node",
			announced: []netip.Prefix{exitRoute},
			approved:  []netip.Prefix{exitRoute},
			selfCaps:  suggestExitNode,
			wantPeerCapMap: tailcfg.NodeCapMap{
				tailcfg.NodeAttrSuggestExitNode: nil,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			node := &types.Node{
				Hostinfo: &tailcfg.Hostinfo{
					RoutableIPs: test.announced,
				},
				ApprovedRoutes: test.approved,
			}

			assert.Equal(t, test.wantPeerCapMap, PeerCapMap(node.View(), test.selfCaps))
		})
	}
}
