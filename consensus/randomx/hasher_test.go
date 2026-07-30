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

	"github.com/ethereum/go-ethereum/common"
)

// TestMeetsTargetUsesLastEightBytesLittleEndian pins the exact comparison
// convention meetsTarget must use: XMRig (and Monero) read a hash's LAST 8
// bytes as a LITTLE-ENDIAN uint64 and compare that against
// maxUint64/difficulty — not the full 32 bytes as a big-endian 256-bit
// integer (the Bitcoin/Ethash convention this engine used before). Using
// the wrong convention doesn't fail loudly: headers still verify (a solo
// miner grinds nonces against whichever rule verifySeal enforces), but a
// pool bridging to XMRig silently loses nearly all block-detection value,
// because a hash meeting an XMRig-style share filter is then statistically
// independent of whether it meets a big-endian network check on the same
// bytes. This was caught in production by a live pool finding zero blocks
// across 26 shares against ~48% expected odds.
func TestMeetsTargetUsesLastEightBytesLittleEndian(t *testing.T) {
	// digest's last 8 bytes, read little-endian, are 0: must meet any target.
	var digest common.Hash
	if !meetsTarget(digest, big.NewInt(1_000_000)) {
		t.Error("all-zero trailing bytes should meet any target")
	}

	// Max possible trailing value: only meets a difficulty-1 target.
	for i := 24; i < 32; i++ {
		digest[i] = 0xff
	}
	if meetsTarget(digest, big.NewInt(2)) {
		t.Error("max trailing value should not meet a difficulty-2 target")
	}
	if !meetsTarget(digest, big.NewInt(1)) {
		t.Error("max trailing value should meet a difficulty-1 target")
	}

	// A large value in the FIRST byte (which would dominate a big-endian
	// full-256-bit comparison) must have zero bearing on the result — only
	// the last 8 bytes matter.
	var digest2 common.Hash
	digest2[0] = 0xff // would fail any big-endian check, must not affect this one
	if !meetsTarget(digest2, big.NewInt(1_000_000)) {
		t.Error("leading bytes must not affect the target comparison")
	}
}

func TestMeetsTargetRejectsNonPositiveOrOversizedDifficulty(t *testing.T) {
	var digest common.Hash
	if meetsTarget(digest, big.NewInt(0)) {
		t.Error("zero difficulty should never meet target")
	}
	if meetsTarget(digest, big.NewInt(-1)) {
		t.Error("negative difficulty should never meet target")
	}
	huge := new(big.Int).Lsh(big.NewInt(1), 65) // exceeds uint64 range
	if meetsTarget(digest, huge) {
		t.Error("a difficulty exceeding uint64 range should never meet target")
	}
}
