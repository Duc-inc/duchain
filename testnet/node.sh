#!/usr/bin/env bash
# Run a non-mining FULL NODE on the testnet, exposing an HTTP JSON-RPC endpoint
# (e.g. for MetaMask / web3 / explorers). Syncs from peers via the bootnodes.
source "$(dirname "${BASH_SOURCE[0]}")/env.sh"

DATADIR="${DATADIR:-$HOME/.duchain-testnet-node}"
HTTP_PORT="${HTTP_PORT:-8545}"

init_if_needed

echo ">> Starting full node on port $PORT, HTTP RPC on :$HTTP_PORT"
exec "$GETH" --datadir "$DATADIR" \
  --networkid "$NETWORK_ID" \
  --port "$PORT" \
  $(bootnodes_flag) $(nat_flag) \
  --http --http.addr 0.0.0.0 --http.port "$HTTP_PORT" \
  --http.api eth,net,web3,txpool \
  --http.corsdomain '*' --http.vhosts '*' \
  --verbosity 3
