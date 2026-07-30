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

package randomx

import (
	crand "crypto/rand"
	"encoding/binary"
	"runtime"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
)

// Seal implements consensus.Engine, attempting to find a nonce that satisfies the
// block's difficulty target. It returns immediately and delivers the sealed block
// on results (or nothing, if aborted via stop).
func (r *RandomX) Seal(chain consensus.ChainHeaderReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error {
	// In fake modes, return the block immediately with a zero solution.
	if r.config.PowMode == ModeFake || r.config.PowMode == ModeFullFake {
		header := block.Header()
		header.Nonce, header.MixDigest = types.BlockNonce{}, common.Hash{}
		select {
		case results <- block.WithSeal(header):
		default:
			r.config.Log.Warn("Sealing result is not read by miner", "mode", "fake")
		}
		return nil
	}

	abort := make(chan struct{})
	found := make(chan *types.Block)

	// Pick a random starting nonce so concurrent miners don't grind the same space.
	var seed [8]byte
	if _, err := crand.Read(seed[:]); err != nil {
		return err
	}
	startNonce := binary.LittleEndian.Uint64(seed[:])

	// Derive the RandomX key once (all threads share it for this block).
	key := seedHash(chain, block.NumberU64())

	threads := r.threads()
	var pend sync.WaitGroup
	for i := 0; i < threads; i++ {
		pend.Add(1)
		go func(id int, nonce uint64) {
			defer pend.Done()
			r.mine(block, key, id, threads, nonce, abort, found)
		}(i, startNonce+uint64(i))
	}

	// Wait for a result or an abort, then tidy up the worker pool.
	go func() {
		var result *types.Block
		select {
		case <-stop:
			close(abort)
		case result = <-found:
			select {
			case results <- result:
			default:
				r.config.Log.Warn("Sealing result is not read by miner", "sealhash", r.SealHash(block.Header()))
			}
			close(abort)
		}
		pend.Wait()
	}()
	return nil
}

// threads returns the configured number of mining threads, defaulting to one per
// CPU when unset.
func (r *RandomX) threads() int {
	n := r.config.Threads
	if n <= 0 {
		n = runtime.NumCPU()
	}
	if n < 1 {
		n = 1
	}
	return n
}

// sealerPool returns the pool of reusable mining hashers, filling it lazily with
// one hasher per mining thread on first use. Reusing these across Seal calls
// keeps the RandomX cache warm: it is only rebuilt when the seed (epoch) changes.
func (r *RandomX) sealerPool() chan Hasher {
	r.sealerInit.Do(func() {
		n := r.threads()
		r.sealers = make(chan Hasher, n)
		for i := 0; i < n; i++ {
			r.sealers <- newHasher(r.config.FullMemory)
		}
	})
	return r.sealers
}

// mine grinds nonces for a single worker, stepping by the worker count so the
// threads partition the search space. It borrows a Hasher from the pool for the
// duration and returns it when done, so the cache survives across blocks.
func (r *RandomX) mine(block *types.Block, key []byte, id, stride int, startNonce uint64, abort <-chan struct{}, found chan<- *types.Block) {
	pool := r.sealerPool()
	hasher := <-pool
	defer func() { pool <- hasher }()

	var (
		header   = block.Header()
		hashSeal = r.SealHash(header)
		nonce    = startNonce
		attempts = int64(0)
	)

	r.config.Log.Trace("Started RandomX search for new nonce", "miner", id, "seed", startNonce)
	for {
		select {
		case <-abort:
			r.config.Log.Trace("RandomX nonce search aborted", "miner", id, "attempts", attempts)
			return
		default:
			attempts++
			digest := hasher.Hash(key, sealInput(hashSeal, nonce))
			if meetsTarget(digest, header.Difficulty) {
				// Found a valid nonce; seal a copy of the header with the solution.
				sealed := types.CopyHeader(header)
				sealed.Nonce = types.EncodeNonce(nonce)
				sealed.MixDigest = digest
				r.config.Log.Trace("RandomX nonce found and reported", "miner", id, "nonce", nonce)
				select {
				case found <- block.WithSeal(sealed):
				case <-abort:
				}
				return
			}
			nonce += uint64(stride)
		}
	}
}
