# Security Review — duchain RandomX fork

Date: 2026-06-18. Scope: the diff against pristine go-ethereum v1.17.3 (RandomX
PoW engine, P2P block gossip + backfill, total-difficulty store + fork-choice,
mining loop, config, testnet scripts).

Threat model: untrusted devp2p peer data is the primary new attack surface; CLI
flags / env vars are trusted operator input.

## In-scope findings (HIGH/MEDIUM exploitable: RCE, auth bypass, injection, invalid-state acceptance)

**None.** Rationale:

- **All peer blocks are validated before affecting state.** Decoded `NewBlock`
  packets and all backfilled blocks go through `BlockChain.InsertChain` →
  `VerifyHeaders`/`verifyHeader`/`verifySeal` (real RandomX PoW: recomputed digest
  must equal `MixDigest` and be ≤ `2²⁵⁶/difficulty`) + transaction re-execution,
  before `writeBlockAndSetHead`. No unvalidated peer data reaches canonical state.
- **TD fork-choice is not forgeable.** `writeBlockAndSetHead` reorgs to a
  non-extending block only if `GetTd(block) > GetTd(head)`; `GetTd` sums
  `header.Difficulty`, but `verifyHeader` rejects any header whose `Difficulty`
  ≠ the LWMA expected value and `verifySeal` requires PoW meeting it. TD is always
  backed by real work; difficulty can't be inflated to force a reorg. Verification
  precedes the fork-choice decision in `insertChain`.
- **Body assembly is safe:** peer bodies are bound to peer headers and validated by
  `InsertChain` root checks; mismatches are rejected.
- **No new injection/crypto/secret/auth surface:** cgo `unsafe.Pointer` only on
  internal fixed-length buffers; `ReadTd` decodes only local DB data; CLI/env
  parsing takes trusted input (`--miner.etherbase` is `IsHexAddress`-validated).

## Out-of-scope observations (DoS / hardening — excluded from formal findings, but real)

1. **Unauthenticated backfill trigger (DoS):** `handleNewBlock` starts backfill on
   `block.Number > head+1` before verifying the announced block's PoW. Should
   PoW-gate announcements. (`eth/handler_eth.go`)
2. **Type assertions on peer responses (DoS):** `fetchHeaders`/`fetchBodies` assert
   response types without `, ok`. Dispatcher matches codes so low-risk; add defensive
   checks. (`eth/handler_eth.go`)
3. **Deep `GetTd` recursion (resource):** recurses to genesis on first call for a high
   block; make iterative for long chains. (`core/blockchain.go`)
4. **`node.sh` RPC on `0.0.0.0` with `corsdomain '*' / vhosts '*'`:** API limited to
   `eth,net,web3,txpool` (no key access), but operators should bind/firewall.

## Conclusion
No exploitable HIGH/MEDIUM security vulnerability in the new code. Consensus-integrity
paths (PoW verification, difficulty validation, TD fork-choice) are sound. Remaining
risks are DoS/hardening class — consistent with the project's pre-production status.
A real public network still requires: anti-DoS hardening, scaled/adversarial testnet
runs, and an independent external audit.
