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
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

// stubChain is a minimal consensus.ChainHeaderReader for tests: it only needs to
// answer GetHeaderByNumber(0) so seedHash can derive a key from "genesis".
type stubChain struct{}

func (stubChain) Config() *params.ChainConfig                 { return params.TestChainConfig }
func (stubChain) CurrentHeader() *types.Header                { return nil }
func (stubChain) GetHeader(common.Hash, uint64) *types.Header { return nil }
func (stubChain) GetHeaderByHash(common.Hash) *types.Header   { return nil }
func (stubChain) GetHeaderByNumber(n uint64) *types.Header {
	return &types.Header{Number: new(big.Int).SetUint64(n), Difficulty: big.NewInt(1)}
}

// newTestHeader builds a minimal header suitable for sealing tests.
func newTestHeader(difficulty int64) *types.Header {
	return &types.Header{
		ParentHash:  types.EmptyRootHash,
		UncleHash:   types.EmptyUncleHash,
		Root:        types.EmptyRootHash,
		TxHash:      types.EmptyTxsHash,
		ReceiptHash: types.EmptyReceiptsHash,
		Number:      big.NewInt(1),
		GasLimit:    8_000_000,
		Time:        100,
		Difficulty:  big.NewInt(difficulty),
	}
}

// TestSealAndVerify mines a low-difficulty block and checks that the resulting
// seal passes verifySeal. Without the `randomx` build tag this exercises the
// keccak stub hasher; with it, the real RandomX binding.
func TestSealAndVerify(t *testing.T) {
	engine := New(Config{PowMode: ModeNormal})
	defer engine.Close()

	block := types.NewBlockWithHeader(newTestHeader(200))

	results := make(chan *types.Block, 1)
	stop := make(chan struct{})
	defer close(stop)

	chain := stubChain{}
	if err := engine.Seal(chain, block, results, stop); err != nil {
		t.Fatalf("seal failed: %v", err)
	}
	select {
	case sealed := <-results:
		header := sealed.Header()
		if err := engine.verifySeal(chain, header); err != nil {
			t.Fatalf("sealed block failed verification: %v", err)
		}
		// A zero nonce/mix would mean nothing was actually solved.
		if (header.Nonce == types.BlockNonce{}) {
			t.Fatal("nonce was not set by the sealer")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("sealing timed out")
	}
}

// TestFakerAcceptsAnySeal ensures the fake engine short-circuits PoW verification.
func TestFakerAcceptsAnySeal(t *testing.T) {
	engine := NewFaker()
	defer engine.Close()

	header := newTestHeader(1 << 30) // absurd difficulty, no real solution
	if err := engine.verifySeal(stubChain{}, header); err != nil {
		t.Fatalf("faker rejected a seal: %v", err)
	}
}

// TestSealHashStable checks that the seal hash ignores the PoW solution fields.
func TestSealHashStable(t *testing.T) {
	engine := NewFaker()
	defer engine.Close()

	header := newTestHeader(200)
	before := engine.SealHash(header)

	header.Nonce = types.EncodeNonce(42)
	header.MixDigest = types.EmptyRootHash
	after := engine.SealHash(header)

	if before != after {
		t.Fatalf("seal hash changed when only nonce/mix changed: %x != %x", before, after)
	}
}
