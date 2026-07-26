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

import "testing"

// TestAnnounceGuardRateLimit checks the token bucket: a full burst passes, the
// next announcements are throttled (ignored), and sustained flooding past the
// hard debt becomes a bannable offence.
func TestAnnounceGuardRateLimit(t *testing.T) {
	g := newAnnounceGuard()
	const peer = "peer-1"

	// The initial burst is allowed in full.
	for i := 0; i < announceBurst; i++ {
		if ok, hard := g.allowAnnounce(peer); !ok || hard {
			t.Fatalf("announcement %d of burst rejected (ok=%v hard=%v)", i+1, ok, hard)
		}
	}
	// Beyond the burst: throttled but not yet bannable.
	if ok, hard := g.allowAnnounce(peer); ok || hard {
		t.Fatalf("expected soft throttle after burst (ok=%v hard=%v)", ok, hard)
	}
	// Keep flooding until the debt runs out: must eventually turn hard.
	hardSeen := false
	for i := 0; i < announceHardDebt+1; i++ {
		if _, hard := g.allowAnnounce(peer); hard {
			hardSeen = true
			break
		}
	}
	if !hardSeen {
		t.Fatal("sustained flood never became a hard violation")
	}
	// An independent peer is unaffected.
	if ok, _ := g.allowAnnounce("peer-2"); !ok {
		t.Fatal("second peer throttled by first peer's flood")
	}
}

// TestAnnounceGuardStrikes checks that repeated soft violations within the
// window cross the ban threshold exactly at violationLimit.
func TestAnnounceGuardStrikes(t *testing.T) {
	g := newAnnounceGuard()
	const peer = "peer-1"

	for i := 1; i < violationLimit; i++ {
		if g.strike(peer) {
			t.Fatalf("strike %d already crossed the limit of %d", i, violationLimit)
		}
	}
	if !g.strike(peer) {
		t.Fatalf("strike %d did not cross the limit", violationLimit)
	}
}

// TestAnnounceGuardBan checks ban bookkeeping: banned peers stay banned across
// forget (disconnect must not reset the sentence), others are unaffected.
func TestAnnounceGuardBan(t *testing.T) {
	g := newAnnounceGuard()

	if g.isBanned("peer-1") {
		t.Fatal("fresh peer reported banned")
	}
	g.ban("peer-1")
	if !g.isBanned("peer-1") {
		t.Fatal("banned peer not reported banned")
	}
	g.forget("peer-1") // disconnect: rate state dropped, ban kept
	if !g.isBanned("peer-1") {
		t.Fatal("ban was lost on disconnect")
	}
	if g.isBanned("peer-2") {
		t.Fatal("unrelated peer reported banned")
	}
}
