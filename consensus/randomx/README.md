# consensus/randomx

A proof-of-work consensus engine for go-ethereum based on **RandomX** (the
CPU-friendly, ASIC-resistant PoW used by Monero). It keeps the standard Ethereum
EVM / state machine and swaps the seal + difficulty for RandomX.

## Files

| File               | Role                                                                    |
|--------------------|-------------------------------------------------------------------------|
| `randomx.go`       | `RandomX` engine type, constructors, `Author/Prepare/Finalize/SealHash` |
| `consensus.go`     | `VerifyHeader(s)` + `verifyHeader` + `verifySeal` (the PoW check)        |
| `difficulty.go`    | `CalcDifficulty` — LWMA retarget (no difficulty bomb)                    |
| `sealer.go`        | `Seal` — the multi-threaded nonce-grinding mining loop                   |
| `hasher.go`        | `Hasher` interface + seed/epoch + seal-input packing                    |
| `bindings_cgo.go`  | **Production** RandomX via cgo → librandomx (`-tags randomx`)            |
| `bindings_stub.go` | keccak fallback so it builds/tests without the C++ lib (`!randomx`)      |
| `randomx_test.go`  | mines a low-difficulty block and verifies the seal                      |

## How it fits the `consensus.Engine` interface

`RandomX` implements every method of `consensus.Engine`. Sealing flow:

1. `Prepare` sets `header.Difficulty` from the LWMA retarget (`difficulty.go`).
2. `Seal` (`sealer.go`) spins up one worker per CPU; each grinds nonces, hashing
   `SealHash(header) || nonce` through RandomX under the epoch's seed key.
3. A nonce wins when `RandomXHash <= 2²⁵⁶ / difficulty`. The winning header gets
   `Nonce` + `MixDigest` set and is returned.
4. `verifySeal` (`consensus.go`) recomputes the hash and re-checks the target.

The seed key is constant within an epoch (`epochLength = 2048` blocks) so the
expensive RandomX cache is built once and reused across nonces.

## Building with real RandomX (production)

The default build uses the keccak **stub** (NOT RandomX). For a real network you
must build librandomx and compile with `-tags randomx`.

```bash
# 1. Build librandomx (the reference C++ implementation)
git clone https://github.com/tevador/RandomX
cd RandomX && mkdir build && cd build
cmake -DARCH=native ..
make
sudo make install            # installs headers + librandomx to /usr/local

# 2. Build geth with the cgo binding active
cd /home/adminus/Desktop/duchain
go build -tags randomx ./...
```

If you installed librandomx somewhere other than `/usr/local`, edit the `#cgo`
`CFLAGS`/`LDFLAGS` lines at the top of `bindings_cgo.go`.

## Tests

```bash
go test ./consensus/randomx/            # stub hasher, fast
go test -tags randomx ./consensus/randomx/   # real RandomX (needs librandomx)
```

## Phase 2 — wiring it into Geth (NOT done yet)

The engine is self-contained and tested, but Geth post-merge will not *use* it
until three things are wired up. These are intentionally left as the next step
because they touch shared files:

1. **`params.ChainConfig`** — add a `RandomX *RandomXConfig` field (mirroring
   `Clique`) so a genesis can select this engine.
2. **`eth/ethconfig/config.go` → `CreateConsensusEngine`** — currently it errors
   unless `terminalTotalDifficulty` is set and wraps everything in
   `beacon.New(...)`. Add a branch: if `config.RandomX != nil`, return
   `randomx.New(...)` directly (no beacon wrapper, no TTD requirement).
3. **The mining loop** — post-merge, `miner/` builds payloads and never calls
   `engine.Seal`. To actually mine locally you must restore a sealing loop that
   feeds the assembled block to `Seal` and imports the sealed result. This is the
   biggest change and effectively reverts the PoW removal in the miner.

Until phase 2 is done, exercise the engine through its unit tests.
