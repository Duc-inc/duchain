#!/usr/bin/env bash
# Shared configuration for the duchain RandomX mainnet.
# Source this from the other scripts: `source env.sh`.

set -euo pipefail

# Absolute path to the geth binary built with `-tags randomx` (see ../testnet/build.sh
# — the binary itself is generic, only the genesis/network below differ).
GETH="${GETH:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/build/geth-randomx}"

# Network identity. Baked into the binary as params.MainnetChainConfig, same
# as how upstream geth boots real Ethereum mainnet with zero flags — no
# genesis.json to distribute or keep in sync (unlike testnet/, which is
# file-based on purpose for easier iteration).
NETWORK_ID="${NETWORK_ID:-271017}"

# Comma-separated bootnode enodes that new nodes dial to discover the network.
# Fill this in with your bootnode's enode (printed by bootnode.sh), including the
# public IP, e.g.:
#   BOOTNODES="enode://<pubkey>@203.0.113.10:30303"
BOOTNODES="${BOOTNODES:-}"

# P2P port and data directory (override per-host / per-role as needed).
PORT="${PORT:-30303}"
DATADIR="${DATADIR:-$HOME/.duchain}"

# Public IP for this host so peers can reach us. Leave empty on a LAN/dev box.
EXTIP="${EXTIP:-}"

init_if_needed() {
  : # No-op: geth bootstraps params.MainnetChainConfig's embedded genesis
    # automatically on first start when no genesis.json was ever `init`-ed.
}

nat_flag() {
  if [ -n "$EXTIP" ]; then echo "--nat extip:$EXTIP"; fi
}

bootnodes_flag() {
  if [ -n "$BOOTNODES" ]; then echo "--bootnodes $BOOTNODES"; fi
}
