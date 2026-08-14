package mapper

import (
	"fmt"
	"net/netip"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juanfont/headscale/hscontrol/policy"
	"github.com/juanfont/headscale/hscontrol/policy/matcher"
	"github.com/juanfont/headscale/hscontrol/routes"
	"github.com/juanfont/headscale/hscontrol/types"
	"tailscale.com/tailcfg"
	"tailscale.com/types/dnstype"
)

var iap = func(ipStr string) *netip.Addr {
	ip := netip.MustParseAddr(ipStr)
	return &ip
}

func TestDNSConfigMapResponse(t *testing.T) {
	tests := []struct {
		magicDNS bool
		want     *tailcfg.DNSConfig
	}{
		{
			magicDNS: true,
			want: &tailcfg.DNSConfig{
				Routes: map[string][]*dnstype.Resolver{},
				Domains: []string{
					"foobar.headscale.net",
				},
				Proxied: true,
			},
		},
		{
			magicDNS: false,
			want: &tailcfg.DNSConfig{
				Domains: []string{"foobar.headscale.net"},
				Proxied: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("with-magicdns-%v", tt.magicDNS), func(t *testing.T) {
			mach := func(hostname, username string, userid uint) *types.Node {
				return &types.Node{
					Hostname: hostname,
					UserID:   userid,
					User: types.User{
						Name: username,
					},
				}
			}

			baseDomain := "foobar.headscale.net"

			dnsConfigOrig := tailcfg.DNSConfig{
				Routes:  make(map[string][]*dnstype.Resolver),
				Domains: []string{baseDomain},
				Proxied: tt.magicDNS,
			}

			nodeInShared1 := mach("test_get_shared_nodes_1", "shared1", 1)

			got := generateDNSConfig(
				&types.Config{
					TailcfgDNSConfig: &dnsConfigOrig,
				},
				nodeInShared1.View(),
				nil,
			)

			if diff := cmp.Diff(tt.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("expandAlias() unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNextDNSCapMapRendering(t *testing.T) {
	t.Parallel()

	mkConfig := func(addrs ...string) *types.Config {
		resolvers := make([]*dnstype.Resolver, len(addrs))
		for index, addr := range addrs {
			resolvers[index] = &dnstype.Resolver{Addr: addr}
		}

		return &types.Config{
			TailcfgDNSConfig: &tailcfg.DNSConfig{Resolvers: resolvers},
		}
	}

	mkNode := func() types.NodeView {
		return (&types.Node{
			ID:       1,
			Hostname: "node1",
			IPv4:     iap("100.64.0.1"),
			Hostinfo: &tailcfg.Hostinfo{OS: "linux"},
		}).View()
	}

	resolverAddr := func(t *testing.T, got *tailcfg.DNSConfig) string {
		t.Helper()
		if got == nil {
			t.Fatal("generateDNSConfig returned nil")
		}
		if len(got.Resolvers) == 0 {
			t.Fatal("generateDNSConfig returned no resolvers")
		}

		return got.Resolvers[0].Addr
	}

	t.Run("no_capmap_metadata_appended", func(t *testing.T) {
		t.Parallel()

		got := generateDNSConfig(
			mkConfig("https://dns.nextdns.io/abc"),
			mkNode(),
			nil,
		)

		want := "https://dns.nextdns.io/abc?device_ip=100.64.0.1&device_model=linux&device_name=node1"
		if addr := resolverAddr(t, got); addr != want {
			t.Errorf("addr = %q, want %q", addr, want)
		}
	})

	t.Run("profile_overrides_global", func(t *testing.T) {
		t.Parallel()

		got := generateDNSConfig(
			mkConfig("https://dns.nextdns.io/global"),
			mkNode(),
			tailcfg.NodeCapMap{"nextdns:override": []tailcfg.RawMessage{}},
		)

		want := "https://dns.nextdns.io/override?device_ip=100.64.0.1&device_model=linux&device_name=node1"
		if addr := resolverAddr(t, got); addr != want {
			t.Errorf("addr = %q, want %q", addr, want)
		}
	})

	t.Run("no_device_info_skips_metadata", func(t *testing.T) {
		t.Parallel()

		got := generateDNSConfig(
			mkConfig("https://dns.nextdns.io/global"),
			mkNode(),
			tailcfg.NodeCapMap{
				"nextdns:abc":            []tailcfg.RawMessage{},
				"nextdns:no-device-info": []tailcfg.RawMessage{},
			},
		)

		if addr := resolverAddr(t, got); addr != "https://dns.nextdns.io/abc" {
			t.Errorf("addr = %q, want %q", addr, "https://dns.nextdns.io/abc")
		}
	})

	t.Run("non_nextdns_resolver_untouched", func(t *testing.T) {
		t.Parallel()

		got := generateDNSConfig(
			mkConfig("https://dns.example.org/dns-query"),
			mkNode(),
			tailcfg.NodeCapMap{"nextdns:abc": []tailcfg.RawMessage{}},
		)

		want := "https://dns.example.org/dns-query"
		if addr := resolverAddr(t, got); addr != want {
			t.Errorf("non-nextdns resolver was rewritten: %q", addr)
		}
	})
}

// mockState is a mock implementation that provides the required methods.
type mockState struct {
	polMan  policy.PolicyManager
	derpMap *tailcfg.DERPMap
	primary *routes.PrimaryRoutes
	nodes   types.Nodes
	peers   types.Nodes
}

func (m *mockState) DERPMap() *tailcfg.DERPMap {
	return m.derpMap
}

func (m *mockState) Filter() ([]tailcfg.FilterRule, []matcher.Match) {
	if m.polMan == nil {
		return tailcfg.FilterAllowAll, nil
	}
	return m.polMan.Filter()
}

func (m *mockState) SSHPolicy(node types.NodeView) (*tailcfg.SSHPolicy, error) {
	if m.polMan == nil {
		return nil, nil
	}
	return m.polMan.SSHPolicy(node)
}

func (m *mockState) NodeCanHaveTag(node types.NodeView, tag string) bool {
	if m.polMan == nil {
		return false
	}
	return m.polMan.NodeCanHaveTag(node, tag)
}

func (m *mockState) GetNodePrimaryRoutes(nodeID types.NodeID) []netip.Prefix {
	if m.primary == nil {
		return nil
	}
	return m.primary.PrimaryRoutes(nodeID)
}

func (m *mockState) ListPeers(nodeID types.NodeID, peerIDs ...types.NodeID) (types.Nodes, error) {
	if len(peerIDs) > 0 {
		// Filter peers by the provided IDs
		var filtered types.Nodes
		for _, peer := range m.peers {
			if slices.Contains(peerIDs, peer.ID) {
				filtered = append(filtered, peer)
			}
		}

		return filtered, nil
	}
	// Return all peers except the node itself
	var filtered types.Nodes
	for _, peer := range m.peers {
		if peer.ID != nodeID {
			filtered = append(filtered, peer)
		}
	}

	return filtered, nil
}

func (m *mockState) ListNodes(nodeIDs ...types.NodeID) (types.Nodes, error) {
	if len(nodeIDs) > 0 {
		// Filter nodes by the provided IDs
		var filtered types.Nodes
		for _, node := range m.nodes {
			if slices.Contains(nodeIDs, node.ID) {
				filtered = append(filtered, node)
			}
		}

		return filtered, nil
	}

	return m.nodes, nil
}

func Test_fullMapResponse(t *testing.T) {
	t.Skip("Test needs to be refactored for new state-based architecture")
	// TODO: Refactor this test to work with the new state-based mapper
	// The test architecture needs to be updated to work with the state interface
	// instead of the old direct dependency injection pattern
}
