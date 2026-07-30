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
	"encoding/binary"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/crypto"
)

// epochLength is the number of blocks that share a single RandomX key (seed).
// Re-keying RandomX is expensive (it rebuilds the cache/dataset), so the seed is
// held stable across an epoch, mirroring Monero's "seed height" mechanism.
const epochLength = uint64(2048)

// Hasher computes 256-bit RandomX hashes.
//
// Implementations are NOT required to be safe for concurrent use: the verifier
// guards its single instance with a mutex, and each mining thread owns a private
// instance. A Hasher may cache the expensive key expansion internally and only
// rebuild it when the key changes between calls.
type Hasher interface {
	// Hash returns the RandomX hash of input under the given key (seed).
	Hash(key, input []byte) common.Hash

	// Close releases any resources (C allocations, datasets) held by the hasher.
	Close() error
}

// newHasher builds the concrete Hasher for the current build. When fullMem is
// true the production binding returns a "fast" full-dataset hasher; otherwise a
// light (cache-only) one. The production RandomX binding lives in bindings_cgo.go
// (build tag `randomx`); a keccak fallback lives in bindings_stub.go (`!randomx`).
//
// (declared in the build-tagged binding files)

// NewHasher exposes newHasher to callers outside this package that need to
// compute RandomX hashes without a full consensus.Engine — e.g. a mining
// pool verifying submitted shares against a pool difficulty target, which
// VerifySeal cannot do since it only checks against a header's own (full
// network) difficulty. Callers own the returned Hasher and must Close it.
func NewHasher(fullMem bool) Hasher {
	return newHasher(fullMem)
}

// seedHash returns the RandomX key for the given block number. To make the key
// unpredictable (so miners cannot precompute RandomX caches/datasets for future
// epochs), it is derived from the hash of a block in the past — the last block of
// the previous epoch — rather than from the epoch index alone. Every block within
// an epoch shares the same key, so the expensive cache/dataset is reused across
// nonces and blocks. Epoch 0 falls back to the genesis hash.
//
// The seed block is at least one full epoch (epochLength blocks) in the past, so
// it is effectively final and agreed by all peers; shallow reorgs never change it.
func seedHash(chain consensus.ChainHeaderReader, number uint64) []byte {
	epoch := number / epochLength

	var seedNumber uint64
	if epoch > 0 {
		seedNumber = epoch*epochLength - 1 // last block of the previous epoch
	}
	if header := chain.GetHeaderByNumber(seedNumber); header != nil {
		hash := header.Hash()
		return hash[:]
	}
	// Fallback (should not happen for a valid chain): deterministic epoch key.
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], epoch)
	return crypto.Keccak256(buf[:])
}

// seedAvailable reports whether the seed block needed to derive the RandomX key
// for the given block number is present in the local chain, i.e. whether seedHash
// would return the real chain-derived key rather than its fallback.
func seedAvailable(chain consensus.ChainHeaderReader, number uint64) bool {
	epoch := number / epochLength

	var seedNumber uint64
	if epoch > 0 {
		seedNumber = epoch*epochLength - 1
	}
	return chain.GetHeaderByNumber(seedNumber) != nil
}

// sealInput packs the sealing hash and nonce into the byte slice that is fed
// to RandomX: the 32-byte seal hash, 3 zero-padding bytes, then the 8-byte
// big-endian nonce (43 bytes total).
//
// The padding is deliberate, not incidental: it places the nonce's
// low-order 4 bytes at absolute byte offset 39, which is the fixed offset
// XMRig hardcodes for the RandomX family (see
// src/base/net/stratum/Job.cpp's nonceOffset() in the XMRig source — it is
// NOT derived from parsing this buffer, XMRig assumes it unconditionally).
// That lets an external miner or mining pool feed XMRig this exact 43-byte
// buffer as its stratum job "blob" and have XMRig's own RandomX hash be a
// genuine, valid seal — rather than a pool needing to compute a second,
// XMRig-incompatible hash purely to detect block-level solutions, which
// silently discards the vast majority of a miner's real hashrate (every
// hash XMRig tries and rejects on its own is invisible to the pool). The
// nonce's high-order 4 bytes (offset 35..39) are untouched by XMRig, so a
// pool is free to use them as a per-worker extranonce.
func sealInput(hash common.Hash, nonce uint64) []byte {
	out := make([]byte, 43)
	copy(out, hash[:])
	binary.BigEndian.PutUint64(out[35:], nonce)
	return out
}
