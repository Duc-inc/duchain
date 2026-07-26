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

package miner

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
)

// MineRandomX runs the local proof-of-work mining loop for RandomX networks. It
// repeatedly assembles a block on top of the current head, seals it through the
// consensus engine, and imports the sealed block into the chain. It blocks until
// exit is closed.
//
// This is the pre-merge sealing path, intentionally revived for RandomX: the
// post-merge PoS path builds payloads via BuildPayload and is driven by a
// consensus client, never reaching this loop.
func (miner *Miner) MineRandomX(coinbase common.Address, exit <-chan struct{}) {
	if coinbase == (common.Address{}) {
		log.Warn("RandomX mining started without an etherbase; block rewards will be sent to the zero address")
	}
	log.Info("Starting RandomX proof-of-work mining", "etherbase", coinbase)
	defer log.Info("RandomX mining stopped")

	for {
		select {
		case <-exit:
			return
		default:
		}

		// Assemble a fresh block on top of the current head.
		work := miner.buildRandomXWork(coinbase)
		if work == nil || work.err != nil {
			if work != nil {
				log.Warn("Failed to assemble RandomX block", "err", work.err)
			}
			if sleepOrDone(time.Second, exit) {
				return
			}
			continue
		}
		block := work.block

		// Kick off sealing. Abort if a competing block extends the chain first, or
		// if the miner is shutting down.
		results := make(chan *types.Block, 1)
		stop := make(chan struct{})
		if err := miner.engine.Seal(miner.chain, block, results, stop); err != nil {
			log.Warn("Failed to start RandomX sealing", "err", err)
			close(stop)
			if sleepOrDone(time.Second, exit) {
				return
			}
			continue
		}

		headCh := make(chan core.ChainHeadEvent, 1)
		sub := miner.chain.SubscribeChainHeadEvent(headCh)

		select {
		case sealed := <-results:
			sub.Unsubscribe()
			if _, err := miner.chain.InsertChain(types.Blocks{sealed}); err != nil {
				log.Warn("Failed to import mined RandomX block", "number", sealed.NumberU64(), "err", err)
			} else {
				log.Info("Successfully sealed new RandomX block",
					"number", sealed.NumberU64(),
					"hash", sealed.Hash(),
					"difficulty", sealed.Difficulty(),
					"txs", len(sealed.Transactions()),
				)
			}
		case <-headCh:
			// The chain advanced underneath us; abandon this work and rebuild on
			// the new head.
			close(stop)
			sub.Unsubscribe()
		case <-exit:
			close(stop)
			sub.Unsubscribe()
			return
		}
	}
}

// buildRandomXWork assembles (but does not seal) a block on top of the current
// head, crediting coinbase with the eventual block reward.
func (miner *Miner) buildRandomXWork(coinbase common.Address) *newPayloadResult {
	parent := miner.chain.CurrentBlock()
	timestamp := uint64(time.Now().Unix())
	if parent.Time >= timestamp {
		timestamp = parent.Time + 1
	}
	return miner.generateWork(context.Background(), &generateParams{
		timestamp:  timestamp,
		parentHash: parent.Hash(),
		coinbase:   coinbase,
		noTxs:      false,
	}, false)
}

// sleepOrDone waits for d to elapse, returning false, or returns true early if
// exit is closed.
func sleepOrDone(d time.Duration, exit <-chan struct{}) bool {
	select {
	case <-time.After(d):
		return false
	case <-exit:
		return true
	}
}
