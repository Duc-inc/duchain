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

//go:build !randomx

package randomx

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// stubHasher is a NON-PRODUCTION fallback used when the `randomx` build tag is
// absent (i.e. librandomx is not linked). It lets the package build and the test
// suite mine at low difficulty without the C++ dependency.
//
// WARNING: this is plain keccak, NOT RandomX. It provides none of RandomX's
// memory-hardness or ASIC resistance. Never run a real network with it — always
// build the node with `-tags randomx` so the cgo binding in bindings_cgo.go is
// used instead.
type stubHasher struct{}

// newHasher returns the keccak fallback hasher. The fullMem flag is ignored:
// the stub has no dataset mode.
func newHasher(fullMem bool) Hasher { return stubHasher{} }

// Hash returns keccak256(key || input). Deterministic and dependency-free, but
// not RandomX.
func (stubHasher) Hash(key, input []byte) common.Hash {
	return common.BytesToHash(crypto.Keccak256(key, input))
}

// Close is a no-op for the stub hasher.
func (stubHasher) Close() error { return nil }
