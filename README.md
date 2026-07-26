# duchain

**duchain** is a fork of [go-ethereum](https://github.com/ethereum/go-ethereum)
(currently rebased on **v1.17.4**) that replaces the post-merge Proof-of-Stake
consensus with a standalone **RandomX Proof-of-Work** engine. It keeps the
standard Ethereum EVM, state machine, and JSON-RPC/tooling, and runs as an
independent, PoW-secured chain — no beacon chain, no consensus client, no
staking.

The mainnet built with this code is **Ducros** (ticker **DUC**, chainId
**271017**) — see [`mainnet/README.md`](mainnet/README.md). A separate
`testnet/` (chainId 61102) exists for development/testing.

> ⚠️ **Status: unaudited.** Do not attach real economic value without reading
> [`SECURITY_REVIEW.md`](SECURITY_REVIEW.md) for the current security posture.

## What's different from upstream go-ethereum

- **RandomX consensus engine** (`consensus/randomx/`) — the CPU-friendly,
  ASIC-resistant PoW used by Monero, swapped in for Ethash/PoS. See
  [`consensus/randomx/README.md`](consensus/randomx/README.md) for how it
  implements `consensus.Engine` and how to build against real `librandomx`.
- **LWMA difficulty retarget** — no difficulty bomb, ~12s target block time,
  60-block averaging window.
- **Total-difficulty fork-choice** — reorgs happen on heaviest-chain (summed
  PoW), like pre-merge Ethereum, instead of LMD-GHOST/finality.
- **Fixed block reward with halving** — 2 coins per block, halving every
  2,100,000 blocks (Bitcoin-style), reaching zero after 64 halvings.
- **Optional treasury fee split** — genesis can route a fixed percentage of
  the priority fee and/or base fee to configured treasury addresses instead of
  paying it all to the miner / burning it. Opt-in, off by default.
- **Validator pinning** (`core/types/validator_pin.go`) — a transaction can pin
  itself to a specific validator/coinbase, making it valid only inside a block
  mined by that exact address.
- **Announcement hardening** (`eth/announce_guard.go`) — guards against
  malicious/premature block and transaction announcements from peers.
- **Multi-node testnet tooling** (`testnet/`) — ready-to-run bootnode/miner/
  node scripts for standing up a small public or private RandomX network.

## Quick start

```bash
go build -tags randomx ./...      # requires librandomx, see consensus/randomx/README.md
```

- **Mainnet (Ducros / DUC)**: see [`mainnet/README.md`](mainnet/README.md). No
  genesis file needed — the binary boots Ducros by default, same as upstream
  geth boots real Ethereum mainnet with zero flags.
- **Testnet**: see [`testnet/README.md`](testnet/README.md) — file-based
  genesis (chainId **61102**), meant for development/iteration.

## Security

The RandomX PoW verification and total-difficulty fork-choice paths have been
reviewed and no exploitable HIGH/MEDIUM finding was identified (see
[`SECURITY_REVIEW.md`](SECURITY_REVIEW.md)). Known gaps before any real
deployment: peer-facing anti-DoS hardening is partial, and there has been no
independent external audit.

## License

Same as upstream go-ethereum: the library code is licensed under the
[GNU Lesser General Public License v3.0](COPYING.LESSER), and the binaries
(`cmd/`) under the [GNU General Public License v3.0](COPYING).
