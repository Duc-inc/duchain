// Copyright 2024 duchain
//
// Validator-pinned transactions ("choose your validator").
//
// duchain lets a sender restrict a transaction so it can only be mined by a
// specific validator (the block coinbase). To stay compatible with every
// standard wallet (ethers.js, MetaMask) we do NOT introduce a new EIP-2718
// transaction type. Instead we carry the pinned validator inside the access
// list of an ordinary EIP-1559 (type-2) transaction:
//
//   accessList: [{ address: ValidatorPinAddress, storageKeys: [<validator>] }]
//
// Because the access list is part of the type-2 signing hash, the pinned
// validator is authenticated by the sender's signature for free. The entry
// only "warms" a storage slot in the EVM, which is harmless and leaves
// execution semantics untouched. On other EVM chains the same transaction is
// simply an ordinary transfer with a (slightly more expensive) access list.

package types

import "github.com/ethereum/go-ethereum/common"

// ValidatorPinAddress is the sentinel access-list address that marks a
// validator-pinned transaction. The 20 bytes spell "DuchainValidatorPin!" in
// ASCII so it is recognisable and extremely unlikely to collide with a real
// account.
var ValidatorPinAddress = common.HexToAddress("0x4475636861696e56616c696461746f7250696e21")

// PinnedValidator reports the validator a transaction is pinned to.
//
// It returns (validator, true) when the transaction's access list contains the
// ValidatorPin sentinel with a non-zero validator address. For ordinary
// transactions — and for a sentinel entry that carries no/zero validator — it
// returns (zero, false), meaning the transaction may be mined by any miner.
func PinnedValidator(tx *Transaction) (common.Address, bool) {
	for _, t := range tx.AccessList() {
		if t.Address != ValidatorPinAddress {
			continue
		}
		if len(t.StorageKeys) == 0 {
			return common.Address{}, false
		}
		v := common.BytesToAddress(t.StorageKeys[0][:])
		if v == (common.Address{}) {
			return common.Address{}, false
		}
		return v, true
	}
	return common.Address{}, false
}
