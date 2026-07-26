#!/usr/bin/env bash
# Run a MINING node on the testnet. Mines RandomX blocks toward $ETHERBASE and
# propagates them to peers discovered via the bootnodes.
#
# Usage: ETHERBASE=0xYourAddress ./miner.sh
source "$(dirname "${BASH_SOURCE[0]}")/env.sh"

ETHERBASE="${ETHERBASE:?set ETHERBASE=0x... (the address that receives block rewards)}"
DATADIR="${DATADIR:-$HOME/.duchain-testnet-miner}"

init_if_needed

# On low-RAM hosts, cap mining threads: export GETH_RANDOMX_THREADS=1
# For fastest hashing on a beefy miner: export GETH_RANDOMX_FULLMEM=1 (needs ~2.3GiB)

echo ">> Starting miner on port $PORT, etherbase $ETHERBASE"
exec "$GETH" --datadir "$DATADIR" \
  --networkid "$NETWORK_ID" \
  --port "$PORT" \
  $(bootnodes_flag) $(nat_flag) \
  --mine --miner.etherbase "$ETHERBASE" \
  --verbosity 3
