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
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/misc"
	"github.com/ethereum/go-ethereum/consensus/misc/eip1559"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

// allowedFutureBlockTimeSeconds is the maximum number of seconds a block's
// timestamp may be ahead of the node's clock before it is rejected as a future
// block.
const allowedFutureBlockTimeSeconds = int64(15)

// Engine-specific errors. They are kept private so the rest of the codebase does
// not depend on a particular consensus engine being in use.
var (
	errOlderBlockTime    = errors.New("timestamp older than parent")
	errUnclesUnsupported = errors.New("randomx does not support uncles")
	errInvalidDifficulty = errors.New("non-positive difficulty")
	errInvalidMixDigest  = errors.New("invalid mix digest")
	errInvalidPoW        = errors.New("invalid proof-of-work")
)

// ErrUnknownSeedBlock is returned by VerifySeal when the header's RandomX epoch
// key cannot be derived because the epoch's seed block is not in the local chain
// (e.g. the header is more than an epoch ahead of our head). The seal is then
// neither valid nor invalid: it simply cannot be checked yet.
var ErrUnknownSeedBlock = errors.New("randomx seed block not available")

// VerifySeal checks a header's RandomX proof-of-work in isolation (without its
// ancestors): the mix digest must be the RandomX hash of the seal input and meet
// the target implied by the header's own declared difficulty. It is used by the
// P2P layer to authenticate block announcements before acting on them, so a peer
// cannot trigger work on our side for free. Note that it does NOT validate the
// declared difficulty against the retarget — that requires the ancestors and is
// done by VerifyHeader on import.
func (r *RandomX) VerifySeal(chain consensus.ChainHeaderReader, header *types.Header) error {
	if !seedAvailable(chain, header.Number.Uint64()) {
		return ErrUnknownSeedBlock
	}
	return r.verifySeal(chain, header)
}

// VerifyHeader checks whether a header conforms to the RandomX consensus rules.
func (r *RandomX) VerifyHeader(chain consensus.ChainHeaderReader, header *types.Header) error {
	if r.config.PowMode == ModeFullFake {
		return nil
	}
	// Short circuit if the header is known, or its parent is not.
	number := header.Number.Uint64()
	if chain.GetHeader(header.Hash(), number) != nil {
		return nil
	}
	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	return r.verifyHeader(chain, header, parent, time.Now().Unix())
}

// VerifyHeaders is similar to VerifyHeader but verifies a batch of headers
// concurrently, returning a quit channel to abort and a results channel.
func (r *RandomX) VerifyHeaders(chain consensus.ChainHeaderReader, headers []*types.Header) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))

	if r.config.PowMode == ModeFullFake || len(headers) == 0 {
		for range headers {
			results <- nil
		}
		return abort, results
	}
	unixNow := time.Now().Unix()
	go func() {
		for i, header := range headers {
			var parent *types.Header
			if i == 0 {
				parent = chain.GetHeader(headers[0].ParentHash, headers[0].Number.Uint64()-1)
			} else if headers[i-1].Hash() == headers[i].ParentHash {
				parent = headers[i-1]
			}
			var err error
			if parent == nil {
				err = consensus.ErrUnknownAncestor
			} else {
				err = r.verifyHeader(chain, header, parent, unixNow)
			}
			select {
			case <-abort:
				return
			case results <- err:
			}
		}
	}()
	return abort, results
}

// verifyHeader performs the full RandomX validity check of a header against its
// parent. See the Yellow Paper §4.3.4 "Block Header Validity" for the shared
// Ethereum rules; the proof-of-work check is RandomX-specific.
func (r *RandomX) verifyHeader(chain consensus.ChainHeaderReader, header, parent *types.Header, unixNow int64) error {
	// Extra-data must be of a reasonable size.
	if uint64(len(header.Extra)) > params.MaximumExtraDataSize {
		return fmt.Errorf("extra-data too long: %d > %d", len(header.Extra), params.MaximumExtraDataSize)
	}
	// Timestamp must be sane: not too far in the future, strictly after parent.
	if header.Time > uint64(unixNow+allowedFutureBlockTimeSeconds) {
		return consensus.ErrFutureBlock
	}
	if header.Time <= parent.Time {
		return errOlderBlockTime
	}
	// Difficulty must match the value our retarget expects.
	expected := r.CalcDifficulty(chain, header.Time, parent)
	if expected.Cmp(header.Difficulty) != 0 {
		return fmt.Errorf("invalid difficulty: have %v, want %v", header.Difficulty, expected)
	}
	// Gas limit must be in range and gas used must fit within it.
	if header.GasLimit > params.MaxGasLimit {
		return fmt.Errorf("invalid gasLimit: have %v, max %v", header.GasLimit, params.MaxGasLimit)
	}
	if header.GasUsed > header.GasLimit {
		return fmt.Errorf("invalid gasUsed: have %d, gasLimit %d", header.GasUsed, header.GasLimit)
	}
	// Gas usage / base fee (EIP-1559 once London is active).
	if !chain.Config().IsLondon(header.Number) {
		if header.BaseFee != nil {
			return fmt.Errorf("invalid baseFee before fork: have %d, expected 'nil'", header.BaseFee)
		}
		if err := misc.VerifyGaslimit(parent.GasLimit, header.GasLimit); err != nil {
			return err
		}
	} else if err := eip1559.VerifyEIP1559Header(chain.Config(), parent, header); err != nil {
		return err
	}
	// Block number must be parent + 1.
	if diff := new(big.Int).Sub(header.Number, parent.Number); diff.Cmp(big.NewInt(1)) != 0 {
		return consensus.ErrInvalidNumber
	}
	// RandomX has no uncles.
	if header.UncleHash != types.EmptyUncleHash {
		return errUnclesUnsupported
	}
	// RandomX is a pure-PoW chain: the PoS / post-merge header fields must be absent.
	switch {
	case header.WithdrawalsHash != nil:
		return fmt.Errorf("invalid withdrawalsHash: have %x, expected nil", header.WithdrawalsHash)
	case header.ExcessBlobGas != nil:
		return fmt.Errorf("invalid excessBlobGas: have %d, expected nil", *header.ExcessBlobGas)
	case header.BlobGasUsed != nil:
		return fmt.Errorf("invalid blobGasUsed: have %d, expected nil", *header.BlobGasUsed)
	case header.ParentBeaconRoot != nil:
		return fmt.Errorf("invalid parentBeaconRoot: have %x, expected nil", *header.ParentBeaconRoot)
	case header.SlotNumber != nil:
		return fmt.Errorf("invalid slotNumber: have %x, expected nil", *header.SlotNumber)
	}
	// Finally, the actual proof-of-work.
	return r.verifySeal(chain, header)
}

// verifySeal checks the RandomX proof-of-work of a header: the stored mix digest
// must equal the RandomX hash of the seal input, and that hash, interpreted as a
// 256-bit integer, must not exceed the target derived from the difficulty.
func (r *RandomX) verifySeal(chain consensus.ChainHeaderReader, header *types.Header) error {
	if r.config.PowMode == ModeFake || r.config.PowMode == ModeFullFake {
		return nil
	}
	if header.Difficulty == nil || header.Difficulty.Sign() <= 0 {
		return errInvalidDifficulty
	}
	key := seedHash(chain, header.Number.Uint64())
	input := sealInput(r.SealHash(header), header.Nonce.Uint64())

	r.lock.Lock()
	digest := r.hasher.Hash(key, input)
	r.lock.Unlock()

	if header.MixDigest != digest {
		return errInvalidMixDigest
	}
	target := new(big.Int).Div(two256, header.Difficulty)
	if new(big.Int).SetBytes(digest[:]).Cmp(target) > 0 {
		return errInvalidPoW
	}
	return nil
}
