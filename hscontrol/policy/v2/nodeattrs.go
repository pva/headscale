package v2

import (
	"fmt"
	"maps"
	"net/netip"
	"slices"

	"github.com/juanfont/headscale/hscontrol/types"
	"tailscale.com/tailcfg"
	"tailscale.com/types/views"
	"tailscale.com/util/deephash"
)

// compileNodeAttrs returns the per-node CapMap derived from policy nodeAttrs
// plus the tailnet-wide RandomizeClientPort policy flag.
func (pol *Policy) compileNodeAttrs(
	users types.Users,
	nodes views.Slice[types.NodeView],
) (map[types.NodeID]tailcfg.NodeCapMap, error) {
	empty := map[types.NodeID]tailcfg.NodeCapMap{}

	if pol == nil {
		return empty, nil
	}

	if len(pol.NodeAttrs) == 0 && !pol.RandomizeClientPort {
		return empty, nil
	}

	result := make(map[types.NodeID]tailcfg.NodeCapMap)
	stamp := func(id types.NodeID, attr tailcfg.NodeCapability) {
		capMap, ok := result[id]
		if !ok {
			capMap = tailcfg.NodeCapMap{}
			result[id] = capMap
		}

		// Capabilities without companion data are represented as null on
		// the wire, matching the Tailscale-hosted control plane.
		if _, exists := capMap[attr]; !exists {
			capMap[attr] = nil
		}
	}

	type nodeIPs struct {
		id  types.NodeID
		ips []netip.Addr
	}

	nodeList := make([]nodeIPs, 0, nodes.Len())
	for _, node := range nodes.All() {
		nodeList = append(nodeList, nodeIPs{id: node.ID(), ips: node.IPs()})
	}

	if pol.RandomizeClientPort {
		for _, node := range nodeList {
			stamp(node.id, tailcfg.NodeAttrRandomizeClientPort)
		}
	}

	for _, nodeAttr := range pol.NodeAttrs {
		if len(nodeAttr.Attrs) == 0 {
			continue
		}

		resolved, err := nodeAttr.Targets.Resolve(pol, users, nodes)
		if err != nil {
			return nil, fmt.Errorf("nodeAttrs target %s: %w", nodeAttr.Targets, err)
		}
		if resolved == nil {
			continue
		}

		for _, node := range nodeList {
			if !slices.ContainsFunc(node.ips, resolved.Contains) {
				continue
			}

			for _, attr := range nodeAttr.Attrs {
				stamp(node.id, attr)
			}
		}
	}

	return result, nil
}

// refreshNodeAttrsLocked recompiles nodeAttrs and records nodes whose CapMap
// differs from the previous snapshot. Caller must hold pm.mu.
func (pm *PolicyManager) refreshNodeAttrsLocked() error {
	if pm.pol != nil &&
		len(pm.pol.NodeAttrs) == 0 &&
		!pm.pol.RandomizeClientPort &&
		len(pm.nodeAttrsHashes) == 0 {
		return nil
	}

	newMap, err := pm.pol.compileNodeAttrs(pm.users, pm.nodes)
	if err != nil {
		return fmt.Errorf("compiling nodeAttrs: %w", err)
	}

	newHashes := make(map[types.NodeID]deephash.Sum, len(newMap))
	for id, capMap := range newMap {
		newHashes[id] = deephash.Hash(&capMap)
	}

	seen := make(map[types.NodeID]struct{}, len(newHashes)+len(pm.nodeAttrsHashes))
	var changed []types.NodeID

	for id, hash := range newHashes {
		seen[id] = struct{}{}
		if pm.nodeAttrsHashes[id] != hash {
			changed = append(changed, id)
		}
	}

	for id := range pm.nodeAttrsHashes {
		if _, ok := seen[id]; !ok {
			changed = append(changed, id)
		}
	}

	pm.nodeAttrsMap = newMap
	pm.nodeAttrsHashes = newHashes
	pm.nodeAttrsChanged = append(pm.nodeAttrsChanged, changed...)

	return nil
}

// NodeCapMap returns a defensive copy of the policy-derived CapMap for id.
func (pm *PolicyManager) NodeCapMap(id types.NodeID) tailcfg.NodeCapMap {
	if pm == nil {
		return nil
	}

	pm.mu.RLock()
	defer pm.mu.RUnlock()

	src := pm.nodeAttrsMap[id]
	if len(src) == 0 {
		return nil
	}

	out := make(tailcfg.NodeCapMap, len(src))
	maps.Copy(out, src)

	return out
}

// NodeCapMaps returns a snapshot of all policy-derived per-node CapMaps.
func (pm *PolicyManager) NodeCapMaps() map[types.NodeID]tailcfg.NodeCapMap {
	if pm == nil {
		return nil
	}

	pm.mu.RLock()
	defer pm.mu.RUnlock()

	out := make(map[types.NodeID]tailcfg.NodeCapMap, len(pm.nodeAttrsMap))
	maps.Copy(out, pm.nodeAttrsMap)

	return out
}

// NodesWithChangedCapMap drains the pending nodeAttrs change buffer.
func (pm *PolicyManager) NodesWithChangedCapMap() []types.NodeID {
	if pm == nil {
		return nil
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	out := pm.nodeAttrsChanged
	pm.nodeAttrsChanged = nil

	return out
}
