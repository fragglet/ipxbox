package ipxswitch

import (
	"sync"
	"time"

	"github.com/fragglet/ipxbox/ipx"
)

const (
	broadcastDest = -1
)

type addressData struct {
	lastRXTime time.Time
	portID     int
}

type portData struct {
	addrs map[ipx.HeaderAddr]bool
}

// routingTable stores the mapping table from IPX address to port number.
// We identify which addresses are on which ports by snooping on the source
// address of packets as they are sent.
type routingTable struct {
	mu    sync.RWMutex
	addrs map[ipx.HeaderAddr]*addressData
	ports map[int]*portData
}

// makeKey returns a new HeaderAddr where the socket field is set to zero.
// This is used as a key for address lookup that only considers network and
// node address.
func makeKey(addr *ipx.HeaderAddr) *ipx.HeaderAddr {
	result := &ipx.HeaderAddr{}
	*result = *addr
	result.Socket = 0
	return result
}

// Record saves an address found in the source address field of a packet that
// was received on the given port number.
func (t *routingTable) Record(sourcePort int, src *ipx.HeaderAddr) {
	if src.Addr == ipx.AddrBroadcast || src.Addr == ipx.AddrNull {
		return
	}
	key := makeKey(src)
	t.mu.Lock()
	defer t.mu.Unlock()
	pd, ok := t.ports[sourcePort]
	if !ok {
		return
	}
	ad, ok := t.addrs[*key]
	if !ok {
		pd.addrs[*key] = true
		ad = &addressData{portID: sourcePort}
		t.addrs[*key] = ad
	} else if ad.portID != sourcePort {
		// Another port was marked as the source for this address.
		// Deassociate from other port, and reassign to new port.
		// This can happen if an uplink client disconnects and then
		// reconnects.
		ad.portID = sourcePort
		if otherPD, ok := t.ports[ad.portID]; ok {
			delete(otherPD.addrs, *key)
		}
		pd.addrs[*key] = true
	}
	// TODO: Garbage collection goroutine for stale addresses
	ad.lastRXTime = time.Now()
}

// LookupDest returns a destination port number to send a packet based on the
// given destination address.
func (t *routingTable) LookupDest(dest *ipx.HeaderAddr) int {
	if dest.Addr == ipx.AddrBroadcast {
		return broadcastDest
	}
	key := makeKey(dest)
	t.mu.RLock()
	defer t.mu.RUnlock()
	ad, ok := t.addrs[*key]
	if !ok {
		return broadcastDest
	}
	return ad.portID
}

func (t *routingTable) AddPort(portID int) {
	pd := &portData{
		addrs: make(map[ipx.HeaderAddr]bool),
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ports[portID] = pd
}

func (t *routingTable) DeletePort(portID int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	pd, ok := t.ports[portID]
	if !ok {
		return
	}
	for key := range pd.addrs {
		if ad, ok := t.addrs[key]; ok && ad.portID == portID {
			delete(t.addrs, key)
		}
	}
}

func makeRoutingTable() *routingTable {
	return &routingTable{
		addrs: make(map[ipx.HeaderAddr]*addressData),
		ports: make(map[int]*portData),
	}
}
