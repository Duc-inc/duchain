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
	"errors"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
)

// hashrateExpiry is how long a self-reported hashrate is trusted before being
// dropped from stats, guarding against workers that vanished without notice.
const hashrateExpiry = 3 * time.Minute

// hashrateSample is a self-reported hashrate with the time it was received.
type hashrateSample struct {
	rate uint64
	at   time.Time
}

// loop owns all RemoteSealer state and serializes access to it via channels;
// it is the only goroutine that touches the work cache and hashrate map, so
// none of that state needs a mutex.
func (rs *RemoteSealer) loop() {
	var (
		currentBlock *types.Block // last block assembled by a fetchWork request
		works        = make(map[common.Hash]*types.Block, remoteWorkCacheSize)
		workOrder    []common.Hash // FIFO of keys in works, oldest first
		rates        = make(map[common.Hash]hashrateSample)
	)

	headCh := make(chan core.ChainHeadEvent, 1)
	sub := rs.chain.SubscribeChainHeadEvent(headCh)
	defer sub.Unsubscribe()

	expiryTicker := time.NewTicker(hashrateExpiry)
	defer expiryTicker.Stop()

	for {
		select {
		case req := <-rs.fetchWorkCh:
			if currentBlock == nil || currentBlock.Coinbase() != req.coinbase {
				result := rs.miner.buildRandomXWork(req.coinbase)
				if result == nil || result.err != nil {
					err := errors.New("failed to assemble RandomX work")
					if result != nil && result.err != nil {
						err = result.err
					}
					req.result <- &workResult{err: err}
					continue
				}
				currentBlock = result.block
			}

			header := currentBlock.Header()
			sealHash := rs.engine.SealHash(header)

			if _, cached := works[sealHash]; !cached {
				works[sealHash] = currentBlock
				workOrder = append(workOrder, sealHash)
				if len(workOrder) > remoteWorkCacheSize {
					delete(works, workOrder[0])
					workOrder = workOrder[1:]
				}
			}

			target := targetFromDifficulty(header.Difficulty)
			seed := rs.engine.SeedHash(rs.chain, header.Number.Uint64())
			req.result <- &workResult{work: [3]string{
				sealHash.Hex(),
				common.BytesToHash(seed).Hex(),
				common.BytesToHash(target.Bytes()).Hex(),
			}}

		case req := <-rs.submitWorkCh:
			block, ok := works[req.sealHash]
			if !ok {
				log.Trace("RandomX work submitted but none pending", "sealhash", req.sealHash)
				req.result <- false
				continue
			}
			sealed := types.CopyHeader(block.Header())
			sealed.Nonce = req.nonce
			sealed.MixDigest = req.mixDigest

			if err := rs.engine.VerifySeal(rs.chain, sealed); err != nil {
				log.Warn("Rejected remote RandomX solution", "sealhash", req.sealHash, "err", err)
				req.result <- false
				continue
			}
			finished := block.WithSeal(sealed)
			if _, err := rs.chain.InsertChain(types.Blocks{finished}); err != nil {
				log.Warn("Failed to import remotely-sealed RandomX block", "number", finished.NumberU64(), "err", err)
				req.result <- false
				continue
			}
			log.Info("Successfully sealed new RandomX block via remote sealer",
				"number", finished.NumberU64(), "hash", finished.Hash(), "difficulty", finished.Difficulty())

			delete(works, req.sealHash)
			currentBlock = nil // next fetch must assemble on top of the new head
			req.result <- true

		case req := <-rs.submitRateCh:
			rates[req.id] = hashrateSample{rate: req.rate, at: time.Now()}
			req.result <- true

		case <-headCh:
			// Chain advanced (locally sealed, remotely sealed, or synced from a
			// peer): the cached block is now stale, force reassembly.
			currentBlock = nil

		case <-expiryTicker.C:
			for id, sample := range rates {
				if time.Since(sample.at) > hashrateExpiry {
					delete(rates, id)
				}
			}

		case <-rs.exitCh:
			return
		}
	}
}
