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
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

// errNoEtherbase is returned by GetWork when no PendingFeeRecipient (miner
// etherbase) has been configured, mirroring classic eth_getWork behavior:
// the RPC needs an address to credit the block reward to.
var errNoEtherbase = errors.New("etherbase must be explicitly specified")

// errRemoteSealerUnavailable is returned when the node has not finished
// starting (or is not a RandomX chain) and the remote sealer isn't ready.
var errRemoteSealerUnavailable = errors.New("remote sealer not available")

// MiningAPI exposes the classic eth_getWork / eth_submitWork /
// eth_submitHashrate RPC trio on RandomX (proof-of-work) chains, letting
// external miners and mining pools fetch work and submit solutions without
// running their own node. It is registered under the "eth" namespace (see
// eth/backend.go APIs()) only when the chain's consensus engine is RandomX.
type MiningAPI struct {
	e *Ethereum
}

// NewMiningAPI creates a new MiningAPI instance.
func NewMiningAPI(e *Ethereum) *MiningAPI {
	return &MiningAPI{e}
}

// GetWork returns the current RandomX work item as
// [sealHash, seedHash, target], each a 0x-prefixed 32-byte hex string. The
// block reward is credited to the node's configured PendingFeeRecipient
// (--miner.etherbase).
func (api *MiningAPI) GetWork() ([3]string, error) {
	coinbase := api.e.config.Miner.PendingFeeRecipient
	if coinbase == (common.Address{}) {
		return [3]string{}, errNoEtherbase
	}
	if api.e.remoteSealer == nil {
		return [3]string{}, errRemoteSealerUnavailable
	}
	return api.e.remoteSealer.FetchWork(coinbase)
}

// SubmitWork reports a solution to a previously issued sealHash. It returns
// true only if the sealHash was known and nonce/mixDigest form a valid
// RandomX solution meeting the block's difficulty target; the block is then
// inserted into the local chain and propagated like any other locally-mined
// block.
func (api *MiningAPI) SubmitWork(nonce types.BlockNonce, sealHash, mixDigest common.Hash) bool {
	if api.e.remoteSealer == nil {
		return false
	}
	return api.e.remoteSealer.SubmitWork(nonce, sealHash, mixDigest)
}

// SubmitHashrate records a self-reported hashrate under id, for stats only;
// it has no effect on consensus or work assignment.
func (api *MiningAPI) SubmitHashrate(rate hexutil.Uint64, id common.Hash) bool {
	if api.e.remoteSealer == nil {
		return false
	}
	return api.e.remoteSealer.SubmitHashrate(id, uint64(rate))
}
