// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package params

import "github.com/ethereum/go-ethereum/common"

// MainnetBootnodes are the enode URLs of the P2P bootstrap nodes running on
// the Ducros (duchain RandomX) mainnet.
var MainnetBootnodes = []string{
	"enode://159644888c93d4827f78f29e7a1c2da642770843c2681879cca581e89a50e38861cbc86a1443464e3a2ae8c94ed911072de5b7bf335ebaa121d5c7d4551dad35@135.125.102.6:30303",
}

// MainnetStaticNodes are the enode URLs of known-good Ducros mainnet nodes
// that every node should always try to stay connected to, in addition to
// (not instead of) normal public discovery -- new nodes can still find and
// join through the bootnodes above exactly as usual. duchain has no
// downloader-driven sync (see eth/catchup.go): a node only ever learns of
// blocks it missed via a live peer connection, so on a small network
// dominated by unrelated public devp2p scanner traffic, discovery alone
// wasn't enough to keep these specific nodes meshed -- which is exactly what
// let the network split into two isolated, independently-mined forks for
// over a week after the 2026-08-15 chain reset (root-caused live, see
// 29eb06b70). Unlike bootnodes (used once to bootstrap discovery), these are
// dialed and redialed for the lifetime of the process.
var MainnetStaticNodes = []string{
	"enode://159644888c93d4827f78f29e7a1c2da642770843c2681879cca581e89a50e38861cbc86a1443464e3a2ae8c94ed911072de5b7bf335ebaa121d5c7d4551dad35@135.125.102.6:30303",
	"enode://2bcbfd6fe5fa9c26405df41b7457c74e25903759020b3bdfb0cc57922f7c14f5c62ab6f236ca6ef7f9183f879016930aecb84b4b6712774be693db1053f3c8e0@164.132.85.141:30303",
	"enode://96a513786fe5977d1b99772752ace0769eb2eef99e11c9036bad09175dbb493767ad102b28c70a64e06968c5c5d06023d71c44569b0f60e53373f38c5b4f85f9@164.132.85.142:30303",
}

// HoodiBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Hoodi test network.
var HoodiBootnodes = []string{
}

// HoleskyBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Holesky test network.
var HoleskyBootnodes = []string{
}

// SepoliaBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Sepolia test network.
var SepoliaBootnodes = []string{
}

var V5Bootnodes = []string{
}

const dnsPrefix = "enrtree://AKA3AM6LPBYEUDMVNU3BSVQJ5AD45Y7YPOHJLEF6W26QOE4VTUDPE@"

// KnownDNSNetwork returns the address of a public DNS-based node list for the given
// genesis hash and protocol. See https://github.com/ethereum/discv4-dns-lists for more
// information.
func KnownDNSNetwork(genesis common.Hash, protocol string) string {
	var net string
	switch genesis {
	case MainnetGenesisHash:
		net = "mainnet"
	case SepoliaGenesisHash:
		net = "sepolia"
	case HoleskyGenesisHash:
		net = "holesky"
	case HoodiGenesisHash:
		net = "hoodi"
	default:
		return ""
	}
	return dnsPrefix + protocol + "." + net + ".ethdisco.net"
}
