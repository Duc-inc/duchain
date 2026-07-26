// Copyright 2024 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package eth

import (
	"sync"
	"time"
)

// Anti-DoS tunables for RandomX block announcements. A legitimate peer relays
// roughly one block per target block time (~12 s) plus short bursts during
// reorgs and backfills, so these limits are generous for honest traffic while
// capping what a hostile peer can make us process.
const (
	// announceBurst is the token-bucket capacity: the number of announcements a
	// peer may deliver back-to-back before throttling kicks in.
	announceBurst = 16

	// announceRefill is the sustained rate: one token (announcement) credited
	// per interval, i.e. ~30 announcements per minute steady-state.
	announceRefill = 2 * time.Second

	// announceHardDebt is how far past an empty bucket a peer may keep sending
	// (silently ignored) before the flood itself becomes a bannable offence.
	announceHardDebt = 64

	// violationLimit is the number of soft violations (e.g. invalid backfill
	// data) within violationWindow that gets a peer banned.
	violationLimit = 3

	// violationWindow is the sliding window over which soft violations count.
	violationWindow = 10 * time.Minute

	// banDuration is how long a banned peer is refused reconnection.
	banDuration = 30 * time.Minute
)

// announceGuard tracks per-peer NewBlock announcement rates and protocol
// violations on RandomX networks, and keeps a temporary denylist of peers that
// crossed the line. All methods are safe for concurrent use.
type announceGuard struct {
	mu      sync.Mutex
	buckets map[string]*announceBucket
	banned  map[string]time.Time // peer id -> ban expiry
}

// announceBucket is a token bucket with a strike counter for one peer.
type announceBucket struct {
	tokens     float64   // remaining announcement allowance (may go negative)
	lastRefill time.Time // last time tokens were credited

	strikes     int       // soft violations within the current window
	strikeStart time.Time // start of the current violation window
}

// newAnnounceGuard creates an empty guard.
func newAnnounceGuard() *announceGuard {
	return &announceGuard{
		buckets: make(map[string]*announceBucket),
		banned:  make(map[string]time.Time),
	}
}

// allowAnnounce debits one announcement from the peer's token bucket. It
// returns ok=false when the peer is over its rate (the message should be
// ignored), and hard=true when the peer kept flooding well past the limit
// (the peer should be banned and dropped).
func (g *announceGuard) allowAnnounce(id string) (ok, hard bool) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	b := g.buckets[id]
	if b == nil {
		b = &announceBucket{tokens: announceBurst, lastRefill: now}
		g.buckets[id] = b
	}
	// Credit tokens for the elapsed time, capped at the burst size.
	b.tokens += now.Sub(b.lastRefill).Seconds() / announceRefill.Seconds()
	if b.tokens > announceBurst {
		b.tokens = announceBurst
	}
	b.lastRefill = now

	b.tokens--
	switch {
	case b.tokens >= 0:
		return true, false
	case b.tokens > -announceHardDebt:
		return false, false
	default:
		return false, true
	}
}

// strike records a soft violation (e.g. a failed backfill import) for the peer.
// It returns true when the peer accumulated enough strikes within the window to
// deserve a ban.
func (g *announceGuard) strike(id string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	b := g.buckets[id]
	if b == nil {
		b = &announceBucket{tokens: announceBurst, lastRefill: now}
		g.buckets[id] = b
	}
	if b.strikes == 0 || now.Sub(b.strikeStart) > violationWindow {
		b.strikes, b.strikeStart = 0, now
	}
	b.strikes++
	return b.strikes >= violationLimit
}

// ban puts the peer on the denylist for banDuration. The caller is responsible
// for actually disconnecting it (e.g. by returning an error from the handler).
func (g *announceGuard) ban(id string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.banned[id] = time.Now().Add(banDuration)
}

// isBanned reports whether the peer is currently denylisted, lazily pruning
// expired entries.
func (g *announceGuard) isBanned(id string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	until, found := g.banned[id]
	if !found {
		return false
	}
	if now.After(until) {
		delete(g.banned, id)
		return false
	}
	return true
}

// forget drops the peer's rate/strike state (called on disconnect). Ban entries
// are deliberately kept so a banned peer cannot reset its sentence by
// reconnecting.
func (g *announceGuard) forget(id string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.buckets, id)
}
