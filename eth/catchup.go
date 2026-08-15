// Copyright 2026 The Ducros authors
// This file is part of duchain, a fork of go-ethereum.
//
// duchain is free software: you can redistribute it and/or modify it under
// the terms of the GNU Lesser General Public License as published by the
// Free Software Foundation, either version 3 of the License, or (at your
// option) any later version.

package eth

import (
	"errors"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/log"
)

const (
	// catchupInterval is how often we check whether any peer is ahead of us.
	catchupInterval = 5 * time.Second

	// catchupBatchSize caps how many blocks are requested in a single round,
	// so a node that's far behind catches up gradually across several ticks
	// instead of issuing one huge request.
	catchupBatchSize = 192

	// catchupTimeout bounds how long we wait for a peer to answer a single
	// headers or bodies request before giving up on that round.
	catchupTimeout = 10 * time.Second
)

// catchupLoop periodically checks whether any connected peer is ahead of our
// local chain and, if so, downloads and imports the blocks we're missing.
//
// duchain never runs upstream's downloader-driven sync: new blocks only ever
// reach a node via direct gossip from a peer that happens to be connected at
// the exact moment the block is mined or imported (see minedBroadcastLoop).
// A node that joins the network late, restarts, or merely has its connection
// to an actively-mining peer drop for a few seconds has no way to recover the
// blocks it missed — gossip only ever carries new blocks forward, it never
// fills gaps behind. This loop is that missing gap-filler: it polls peers'
// advertised block range (BlockRangeUpdatePacket, already exchanged over the
// eth wire protocol) and, when behind, pulls headers and bodies directly and
// feeds them to InsertChain, which already implements the total-difficulty
// fork-choice rule — this loop only decides *when* to fetch, never which
// chain wins.
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

// tryCatchup fetches and imports one batch of blocks from whichever connected
// peer reports the highest block, if that peer is ahead of our local head.
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
	if best == nil {
		return
	}

	local := h.chain.CurrentBlock().Number.Uint64()
	if bestBlock <= local {
		return
	}

	from := local + 1
	amount := int(bestBlock - local)
	if amount > catchupBatchSize {
		amount = catchupBatchSize
	}

	headers, err := h.catchupFetchHeaders(best, from, amount)
	if err != nil {
		log.Debug("Catch-up header fetch failed", "peer", best.ID(), "from", from, "err", err)
		return
	}
	if len(headers) == 0 {
		return
	}

	blocks, err := h.catchupFetchBodies(best, headers)
	if err != nil {
		log.Debug("Catch-up body fetch failed", "peer", best.ID(), "from", from, "err", err)
		return
	}

	if _, err := h.chain.InsertChain(blocks); err != nil {
		log.Warn("Catch-up chain insertion failed", "peer", best.ID(), "from", from, "count", len(blocks), "err", err)
		return
	}
	log.Info("Caught up blocks from peer", "peer", best.ID(), "from", from, "count", len(blocks),
		"head", h.chain.CurrentBlock().Number.Uint64())
}

// catchupFetchHeaders requests a batch of headers starting at the given
// block number and blocks until the peer answers or the request times out.
func (h *handler) catchupFetchHeaders(peer *ethPeer, from uint64, amount int) ([]*types.Header, error) {
	resCh := make(chan *eth.Response)
	req, err := peer.RequestHeadersByNumber(from, amount, 0, false, resCh)
	if err != nil {
		return nil, err
	}
	defer req.Close()

	timer := time.NewTimer(catchupTimeout)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil, errors.New("timeout waiting for headers")
	case res := <-resCh:
		headers := *res.Res.(*eth.BlockHeadersRequest)
		res.Done <- nil
		return headers, nil
	}
}

// catchupFetchBodies requests bodies for the given headers, blocks until the
// peer answers or the request times out, and assembles full blocks.
func (h *handler) catchupFetchBodies(peer *ethPeer, headers []*types.Header) (types.Blocks, error) {
	hashes := make([]common.Hash, len(headers))
	for i, header := range headers {
		hashes[i] = header.Hash()
	}

	resCh := make(chan *eth.Response)
	req, err := peer.RequestBodies(hashes, resCh)
	if err != nil {
		return nil, err
	}
	defer req.Close()

	timer := time.NewTimer(catchupTimeout)
	defer timer.Stop()

	var bodies eth.BlockBodiesResponse
	select {
	case <-timer.C:
		return nil, errors.New("timeout waiting for bodies")
	case res := <-resCh:
		bodies = *res.Res.(*eth.BlockBodiesResponse)
		res.Done <- nil
	}
	if len(bodies) != len(headers) {
		return nil, fmt.Errorf("peer sent %d bodies for %d headers", len(bodies), len(headers))
	}

	blocks := make(types.Blocks, len(headers))
	for i, header := range headers {
		txs, err := bodies[i].Transactions.Items()
		if err != nil {
			return nil, fmt.Errorf("body %d: bad transactions: %w", i, err)
		}
		uncles, err := bodies[i].Uncles.Items()
		if err != nil {
			return nil, fmt.Errorf("body %d: bad uncles: %w", i, err)
		}
		var withdrawals []*types.Withdrawal
		if bodies[i].Withdrawals != nil {
			withdrawals, err = bodies[i].Withdrawals.Items()
			if err != nil {
				return nil, fmt.Errorf("body %d: bad withdrawals: %w", i, err)
			}
		}
		blocks[i] = types.NewBlockWithHeader(header).WithBody(types.Body{
			Transactions: txs,
			Uncles:       uncles,
			Withdrawals:  withdrawals,
		})
	}
	return blocks, nil
}
