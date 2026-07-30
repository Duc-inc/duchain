# duchain

**duchain** is a fork of [go-ethereum](https://github.com/ethereum/go-ethereum)
(currently rebased on **v1.17.4**) that replaces the post-merge Proof-of-Stake
consensus with a standalone **RandomX Proof-of-Work** engine. It keeps the
standard Ethereum EVM, state machine, and JSON-RPC/tooling, and runs as an
independent, PoW-secured chain — no beacon chain, no consensus client, no
staking.

The mainnet built with this code is **Ducros** (ticker **DUC**, chainId
**271017**) — its genesis is baked directly into the binary
(`params.MainnetChainConfig`), the same way upstream go-ethereum embeds real
Ethereum mainnet, so there's no launch-scripts folder or genesis file to run:
just `geth-randomx` with plain flags, like real geth. A separate `testnet/`
(chainId 61102, file-based genesis) exists for development/testing.

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
- **Fixed, perpetual block reward** — 9 DUC per block, forever (Ethereum
  PoW-style issuance, no halving, no supply cap).
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

Requires `librandomx` installed — see
[`consensus/randomx/README.md`](consensus/randomx/README.md).

```bash
CGO_ENABLED=1 go build -tags randomx -o build/geth-randomx ./cmd/geth
```

### Ducros (DUC) mainnet

No genesis file, no `--networkid` (it auto-derives from the embedded chainId
271017), no launch scripts — same as running plain `geth` on real Ethereum
mainnet. Open P2P port 30303 (TCP+UDP) on the firewall of any host you run
this on.

```bash
# One-time: only whoever hosts the network's stable entry point runs this.
# Once params.MainnetBootnodes is populated with a real server, nobody else needs to.
./build/geth-randomx --datadir ~/.duchain-boot --port 30303 --nat extip:<public-ip>

# Get its enode (send it to be baked into params/bootnodes.go as MainnetBootnodes):
./build/geth-randomx attach --exec admin.nodeInfo.enode ~/.duchain-boot/geth.ipc

# A mining node (until MainnetBootnodes is populated, add --bootnodes <enode> from above):
./build/geth-randomx --datadir ~/.duchain --port 30303 --nat extip:<this-host-ip> \
  --mine --miner.etherbase 0xYourAddress

# A non-mining full node with JSON-RPC (e.g. for MetaMask/explorers):
./build/geth-randomx --datadir ~/.duchain-node --port 30303 --nat extip:<this-host-ip> \
  --http --http.addr 0.0.0.0 --http.port 8545 --http.api eth,net,web3,txpool
```

- `GETH_RANDOMX_THREADS=N` — mining threads (each light-mode thread holds ~256 MiB). Default: one per CPU.
- `GETH_RANDOMX_FULLMEM=1` — fast full-dataset mining (~2.3 GiB RAM), much faster hashing.

### Testnet

See [`testnet/README.md`](testnet/README.md) — file-based genesis (chainId
**61102**), meant for development/iteration before anything lands on mainnet.

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
