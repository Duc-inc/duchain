# duchain RandomX mainnet

The launch configuration for the duchain RandomX proof-of-work mainnet
(chainId **271017**). These scripts wrap the `geth-randomx` binary; the actual
hosting (servers, public IPs, DNS) is yours to provide.

## 0. Prerequisites
- Go ≥ 1.24 and `librandomx` installed (see `../consensus/randomx/README.md`).
- Open the P2P port (default **30303**, TCP+UDP) on each host's firewall.

## 1. Build
```bash
../testnet/build.sh          # produces ../build/geth-randomx (binary is generic)
```

## 2. Launch a bootnode (one stable, public host)
```bash
EXTIP=<public-ip> ./bootnode.sh
# In another shell, grab its enode:
../build/geth-randomx attach --exec admin.nodeInfo.enode ~/.duchain-boot/geth.ipc
```
Put that enode (with the public IP) into `env.sh` → `BOOTNODES` on every other host.

## 3. Launch miners and nodes (other hosts)
```bash
export BOOTNODES="enode://...@<bootnode-ip>:30303"
EXTIP=<this-host-ip> ETHERBASE=0xYourAddress ./miner.sh   # a mining node
EXTIP=<this-host-ip> ./node.sh                            # an RPC full node
```
A fresh node syncs the chain from peers (header backfill + heaviest-chain fork
choice). New miners need ≥1 announced block to trigger catch-up — fine on a
live network.

## Tuning (node-local env vars)
- `GETH_RANDOMX_THREADS=N` — mining threads (each light-mode thread holds ~256 MiB). Default: one per CPU.
- `GETH_RANDOMX_FULLMEM=1` — fast full-dataset mining (~2.3 GiB RAM), much faster hashing.

## Parameters
- chainId / networkId: **271017**
- Consensus: RandomX PoW, LWMA retarget (target ~12 s/block, window 60)
- Genesis difficulty: `0x800` (self-adjusts via LWMA)
- Block reward: 2 coins, halving every 2,100,000 blocks
- No pre-mine: genesis `alloc` is empty, every coin comes from mining
- Forks: Homestead…London at genesis; no Shanghai/Cancun (PoW chain, no withdrawals/blobs)

## Treasury fee split (active on this network)
A fixed **10%** of every transaction's fees is routed to two treasury addresses
(consensus-critical, baked into `genesis.json` — identical on every node):

- `tipTreasury` — `0xEc4824ADdd1E160De6a13003bD2b815c2Fd969F6` — gets 10% of the priority fee (tip); the miner keeps the remaining 90%.
- `baseFeeTreasury` — `0xf6d08E1255Dbd706C5e824FAC237352564DF987D` — gets 10% of the base fee that would otherwise be burned (EIP-1559); the remaining 90% is still burned as usual.

## ⚠️ Security status
This is the **same, unaudited codebase** as the duchain testnet — running it
as "mainnet" changes the genesis and intent, not the review status. Network
hardening is partial (no anti-DoS peer scoring / announcement rate-limiting
yet), and the low bootstrap difficulty makes early 51% attacks trivial until
enough independent hashrate joins. See `../SECURITY_REVIEW.md` for the current
security posture and open risks before attaching real economic value.
