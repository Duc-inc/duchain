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
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/randomx"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
)

// errNoRandomXEngine is returned when the miner's consensus engine is not
// RandomX; the remote sealer only makes sense on RandomX (PoW) chains.
var errNoRandomXEngine = errors.New("remote sealer requires a RandomX consensus engine")

// errRemoteSealerStopped is returned by the public methods once the sealer's
// background loop has been stopped.
var errRemoteSealerStopped = errors.New("remote sealer is stopped")

// remoteWorkCacheSize bounds how many distinct work items (one per SealHash)
// the remote sealer remembers at once, so a miner that fetched work just
// before a new head still has a short grace window to submit it.
const remoteWorkCacheSize = 8

// two256 is 2^256, used to turn a difficulty into a hash target.
var two256 = new(big.Int).Lsh(big.NewInt(1), 256)

// targetFromDifficulty returns the RandomX hash target (2^256 / difficulty)
// that a sealed header's digest must not exceed, mirroring the check in
// consensus/randomx's verifySeal.
func targetFromDifficulty(diff *big.Int) *big.Int {
	return new(big.Int).Div(two256, diff)
}

// RemoteSealer bridges the RandomX consensus engine to external, RPC-driven
// miners (e.g. a mining pool): it assembles candidate blocks on demand
// (FetchWork) and accepts externally-found solutions (SubmitWork), instead of
// grinding nonces itself like the local Seal() path (see randomx.go) does.
//
// All mutable state is owned by a single goroutine (loop, in
// remote_sealer_loop.go) and manipulated only via channels, so no mutex is
// needed here.
type RemoteSealer struct {
	miner  *Miner
	engine *randomx.RandomX
	chain  *core.BlockChain

	fetchWorkCh  chan *fetchWorkRequest
	submitWorkCh chan *submitWorkRequest
	submitRateCh chan *submitRateRequest
	exitCh       chan struct{}
}

// fetchWorkRequest asks the loop for the current work item, assembling a new
// one for coinbase if none is cached yet.
type fetchWorkRequest struct {
	coinbase common.Address
	result   chan *workResult
}

// workResult is the classic eth_getWork shape: [sealHash, seedHash, target],
// each a 0x-prefixed 32-byte hex string.
type workResult struct {
	work [3]string
	err  error
}

// submitWorkRequest reports a candidate solution to a previously issued
// sealHash.
type submitWorkRequest struct {
	nonce     types.BlockNonce
	sealHash  common.Hash
	mixDigest common.Hash
	result    chan bool
}

// submitRateRequest records a worker's self-reported hashrate for stats.
type submitRateRequest struct {
	id     common.Hash
	rate   uint64
	result chan bool
}

// NewRemoteSealer constructs a RemoteSealer for the given miner and starts
// its background loop. It returns errNoRandomXEngine if the miner's engine is
// not RandomX (the caller should only construct this on RandomX chains).
func NewRemoteSealer(miner *Miner, chain *core.BlockChain) (*RemoteSealer, error) {
	rx, ok := miner.engine.(*randomx.RandomX)
	if !ok {
		return nil, errNoRandomXEngine
	}
	rs := &RemoteSealer{
		miner:        miner,
		engine:       rx,
		chain:        chain,
		fetchWorkCh:  make(chan *fetchWorkRequest),
		submitWorkCh: make(chan *submitWorkRequest),
		submitRateCh: make(chan *submitRateRequest),
		exitCh:       make(chan struct{}),
	}
	go rs.loop()
	return rs, nil
}

// Stop terminates the background loop. It is safe to call at most once.
func (rs *RemoteSealer) Stop() {
	close(rs.exitCh)
}

// FetchWork returns the current (or freshly assembled) work item paying the
// block reward to coinbase, in the classic eth_getWork [sealHash, seedHash,
// target] shape.
func (rs *RemoteSealer) FetchWork(coinbase common.Address) ([3]string, error) {
	req := &fetchWorkRequest{coinbase: coinbase, result: make(chan *workResult, 1)}
	select {
	case rs.fetchWorkCh <- req:
	case <-rs.exitCh:
		return [3]string{}, errRemoteSealerStopped
	}
	res := <-req.result
	return res.work, res.err
}

// SubmitWork reports a solution to a previously issued work item. It returns
// true only if the sealHash was known and the nonce/mixDigest pair is a valid
// RandomX solution meeting the block's difficulty target.
func (rs *RemoteSealer) SubmitWork(nonce types.BlockNonce, sealHash, mixDigest common.Hash) bool {
	req := &submitWorkRequest{nonce: nonce, sealHash: sealHash, mixDigest: mixDigest, result: make(chan bool, 1)}
	select {
	case rs.submitWorkCh <- req:
	case <-rs.exitCh:
		return false
	}
	return <-req.result
}

// SubmitHashrate records a self-reported hashrate under id, for stats only;
// it has no effect on consensus or work assignment.
func (rs *RemoteSealer) SubmitHashrate(id common.Hash, rate uint64) bool {
	req := &submitRateRequest{id: id, rate: rate, result: make(chan bool, 1)}
	select {
	case rs.submitRateCh <- req:
	case <-rs.exitCh:
		return false
	}
	return <-req.result
}
