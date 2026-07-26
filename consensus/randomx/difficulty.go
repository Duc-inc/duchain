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

	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
)

// Difficulty retarget parameters. RandomX uses an LWMA (Linearly Weighted Moving
// Average) algorithm — the same family Monero and other CPU-mined chains use —
// rather than Ethereum's EIP-2/EIP-100 retarget, and there is no difficulty bomb.
const (
	// targetBlockTime is the desired number of seconds between blocks (T).
	targetBlockTime = uint64(12)

	// difficultyWindow is the number of past blocks averaged over (N). Larger is
	// smoother but slower to react to hashrate changes.
	difficultyWindow = uint64(60)
)

var (
	// minimumDifficulty is the floor the retarget will never drop below.
	minimumDifficulty = big.NewInt(1000)

	bigTargetBlockTime = new(big.Int).SetUint64(targetBlockTime)
	big2               = big.NewInt(2)
)

// CalcDifficulty implements consensus.Engine. It returns the difficulty the block
// after parent should have, using the LWMA retarget over the difficulty window.
func (r *RandomX) CalcDifficulty(chain consensus.ChainHeaderReader, time uint64, parent *types.Header) *big.Int {
	return calcDifficultyLWMA(chain, parent)
}

// calcDifficultyLWMA computes the next difficulty from the window of blocks ending
// at parent. The new block's own (attacker-controlled) timestamp is intentionally
// not used, only confirmed historical timestamps, which hardens the retarget
// against timestamp manipulation.
//
// LWMA-1 (zawy):
//
//	next = (Σ Dᵢ) · T · (N+1) / (2 · Σ(i · solveᵢ))
//
// where i ∈ [1,N] weights recent blocks more heavily and solveᵢ is clamped to
// dampen out-of-order or forged timestamps.
func calcDifficultyLWMA(chain consensus.ChainHeaderReader, parent *types.Header) *big.Int {
	parentNumber := parent.Number.Uint64()

	// Not enough history yet: hold the parent's difficulty (or the floor at genesis).
	if parentNumber < difficultyWindow {
		if parent.Difficulty != nil && parent.Difficulty.Sign() > 0 {
			return new(big.Int).Set(parent.Difficulty)
		}
		return new(big.Int).Set(minimumDifficulty)
	}

	// Collect the window [parent-N .. parent], newest first.
	headers := make([]*types.Header, 0, difficultyWindow+1)
	cursor := parent
	headers = append(headers, cursor)
	for i := uint64(0); i < difficultyWindow; i++ {
		ancestor := chain.GetHeader(cursor.ParentHash, cursor.Number.Uint64()-1)
		if ancestor == nil {
			// A gap in the chain we can't span; fall back to the parent difficulty.
			return new(big.Int).Set(parent.Difficulty)
		}
		headers = append(headers, ancestor)
		cursor = ancestor
	}
	// Reverse to chronological order: headers[0] oldest .. headers[N] == parent.
	for i, j := 0, len(headers)-1; i < j; i, j = i+1, j-1 {
		headers[i], headers[j] = headers[j], headers[i]
	}

	var (
		weightedSolve = new(big.Int)
		difficultySum = new(big.Int)
		maxSolve      = int64(targetBlockTime) * 6 // clamp single solve times
	)
	for i := uint64(1); i <= difficultyWindow; i++ {
		solve := int64(headers[i].Time) - int64(headers[i-1].Time)
		if solve < 1 {
			solve = 1
		}
		if solve > maxSolve {
			solve = maxSolve
		}
		// weight = i (linear, favouring recent blocks)
		weightedSolve.Add(weightedSolve, big.NewInt(solve*int64(i)))
		difficultySum.Add(difficultySum, headers[i].Difficulty)
	}
	// Guard against a degenerate window.
	if weightedSolve.Sign() <= 0 {
		return new(big.Int).Set(parent.Difficulty)
	}

	// next = difficultySum · T · (N+1) / (2 · weightedSolve)
	next := new(big.Int).Set(difficultySum)
	next.Mul(next, bigTargetBlockTime)
	next.Mul(next, new(big.Int).SetUint64(difficultyWindow+1))
	next.Div(next, new(big.Int).Mul(big2, weightedSolve))

	if next.Cmp(minimumDifficulty) < 0 {
		next.Set(minimumDifficulty)
	}
	return next
}
