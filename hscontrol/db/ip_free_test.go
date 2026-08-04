package db

import (
	"net/netip"
	"testing"

	"github.com/juanfont/headscale/hscontrol/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIPAllocatorFreeIPs(t *testing.T) {
	prefix := netip.MustParsePrefix("100.64.0.0/30")
	alloc, err := NewIPAllocator(nil, &prefix, nil, types.IPAllocationStrategySequential)
	require.NoError(t, err)

	ip, _, err := alloc.Next()
	require.NoError(t, err)
	require.NotNil(t, ip)

	used, err := alloc.usedIPs.IPSet()
	require.NoError(t, err)
	assert.True(t, used.Contains(*ip))

	alloc.FreeIPs([]netip.Addr{*ip})
	used, err = alloc.usedIPs.IPSet()
	require.NoError(t, err)
	assert.False(t, used.Contains(*ip))
}
