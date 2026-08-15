// Copyright 2026 The Ducros authors
// This file is part of duchain, a fork of go-ethereum.
//
// duchain is free software: you can redistribute it and/or modify it under
// the terms of the GNU Lesser General Public License as published by the
// Free Software Foundation, either version 3 of the License, or (at your
// option) any later version.

package eth

import "time"

// catchupInterval is how often we check whether any connected peer is ahead
// of our local chain.
const catchupInterval = 5 * time.Second

// catchupLoop periodically checks whether any connected peer is ahead of our
// local chain and, if so, triggers the existing RandomX backfill mechanism
// (startBackfill/backfill in handler_eth.go) to pull the missing blocks.
//
// Without this, backfill only ever fires reactively, from inside
// handleNewBlock, when a freshly gossiped block turns out to be more than
// one ahead of our head. A node that's behind but doesn't happen to receive
// a fresh gossiped block while connected — or whose connection to an
// actively-mining peer keeps dropping and re-establishing — never gets that
// trigger and stays stuck indefinitely, since duchain runs no other sync
// path (see f23e14bab). Root-caused live: the RPC node sat at block 0 for
// ~20 minutes with a stable peer at block 186+, because no NewBlock gossip
// ever happened to land during a connected window.
//
// This loop is the missing proactive trigger. It doesn't fetch anything
// itself — it just periodically asks "is any peer ahead of me?" and, if so,
// hands off to the same backfill() that already does proper common-ancestor
// resolution and peer banning on bad data. startBackfill's CompareAndSwap
// guard makes it safe to call repeatedly even if a reactive backfill from
// handleNewBlock is already in flight.
func (h *handler) catchupLoop() {
	defer h.wg.Done()

	ticker := time.NewTicker(catchupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.tryCatchup()
		case <-h.quitSync:
			return
		}
	}
}

// tryCatchup finds whichever connected peer reports the highest block and,
// if it's ahead of our local head, triggers a backfill from it.
func (h *handler) tryCatchup() {
	var (
		best      *ethPeer
		bestBlock uint64
	)
	for _, peer := range h.peers.all() {
		rng := peer.BlockRange()
		if rng == nil {
			continue
		}
		if best == nil || rng.LatestBlock > bestBlock {
			best = peer
			bestBlock = rng.LatestBlock
		}
	}
	if best == nil || bestBlock <= h.chain.CurrentBlock().Number.Uint64() {
		return
	}
	(*ethHandler)(h).startBackfill(best.Peer, bestBlock)
}
