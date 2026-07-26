// Copyright 2024 duchain

package types

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// pinTuple builds a ValidatorPin access-list entry for the given validator.
func pinTuple(validator common.Address) AccessTuple {
	var key common.Hash
	copy(key[12:], validator[:]) // left-pad the 20-byte address into a 32-byte key
	return AccessTuple{Address: ValidatorPinAddress, StorageKeys: []common.Hash{key}}
}

func TestPinnedValidator(t *testing.T) {
	validator := common.HexToAddress("0xD035e5d6C9D4D9373Fd871D0732E0422cEb15a8c")
	other := common.HexToAddress("0x1111111111111111111111111111111111111111")

	tests := []struct {
		name   string
		al     AccessList
		want   common.Address
		pinned bool
	}{
		{"legacy/no-list", nil, common.Address{}, false},
		{"ordinary-access-list", AccessList{{Address: other, StorageKeys: []common.Hash{{}}}}, common.Address{}, false},
		{"pinned", AccessList{pinTuple(validator)}, validator, true},
		{"pinned-among-others", AccessList{{Address: other}, pinTuple(validator)}, validator, true},
		{"sentinel-no-key", AccessList{{Address: ValidatorPinAddress}}, common.Address{}, false},
		{"sentinel-zero-validator", AccessList{{Address: ValidatorPinAddress, StorageKeys: []common.Hash{{}}}}, common.Address{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := NewTx(&DynamicFeeTx{
				ChainID:    big.NewInt(17171),
				Nonce:      0,
				GasFeeCap:  big.NewInt(1),
				GasTipCap:  big.NewInt(1),
				Gas:        21000,
				To:         &other,
				Value:      big.NewInt(1),
				AccessList: tt.al,
			})
			got, ok := PinnedValidator(tx)
			if ok != tt.pinned || got != tt.want {
				t.Fatalf("PinnedValidator() = (%x, %v), want (%x, %v)", got, ok, tt.want, tt.pinned)
			}
		})
	}
}
