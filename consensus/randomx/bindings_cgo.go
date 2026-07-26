// Copyright 2024 The go-ethereum Authors
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

//go:build randomx

// This file holds the PRODUCTION RandomX hasher: cgo bindings to the reference
// C++ library (https://github.com/tevador/RandomX). It is only compiled when the
// `randomx` build tag is set, e.g. `go build -tags randomx ./...`.
//
// Two hasher flavours are provided:
//   - rxHasher: light mode (cache only, ~256 MiB). Used for verification and for
//     mining when full-memory mode is off.
//   - rxDatasetHasher: fast mode (full ~2.3 GiB dataset). Used for mining when
//     full-memory mode is on. All mining threads share a single dataset.
//
// Build prerequisites (see consensus/randomx/README.md): librandomx built and
// installed with its header in the include path and the library in the link
// path. Adjust the #cgo paths below if it is not under /usr/local.

package randomx

/*
#cgo CFLAGS: -I/usr/local/include
#cgo LDFLAGS: -L/usr/local/lib -lrandomx -lstdc++ -lm
#include <stdlib.h>
#include "randomx.h"
*/
import "C"

import (
	"bytes"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ethereum/go-ethereum/common"
)

// newHasher returns the production RandomX hasher. When fullMem is true it
// returns a full-dataset (fast) hasher, otherwise a light (cache-only) hasher.
func newHasher(fullMem bool) Hasher {
	flags := C.randomx_get_flags()
	if fullMem {
		full := C.randomx_flags(uint(flags) | uint(C.RANDOMX_FLAG_FULL_MEM))
		return &rxDatasetHasher{flags: full, base: flags}
	}
	return &rxHasher{flags: flags}
}

// -----------------------------------------------------------------------------
// Light mode (cache only)
// -----------------------------------------------------------------------------

// rxHasher is a light-mode RandomX hasher. It lazily (re)builds its cache and VM
// whenever the key changes, so grinding many nonces under one seed only pays the
// expensive cache initialisation once.
type rxHasher struct {
	mu    sync.Mutex
	flags C.randomx_flags
	cache *C.randomx_cache
	vm    *C.randomx_vm
	key   []byte
}

// Hash returns the RandomX hash of input under key, rebuilding the VM if the key
// changed since the previous call.
func (h *rxHasher) Hash(key, input []byte) common.Hash {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.vm == nil || !bytes.Equal(h.key, key) {
		h.rekey(key)
	}
	var out [32]byte
	C.randomx_calculate_hash(
		h.vm,
		unsafe.Pointer(&input[0]), C.size_t(len(input)),
		unsafe.Pointer(&out[0]),
	)
	return common.Hash(out)
}

// rekey tears down the current cache/VM and rebuilds them for the new key. Must
// be called with h.mu held.
func (h *rxHasher) rekey(key []byte) {
	h.release()
	h.cache = C.randomx_alloc_cache(h.flags)
	if h.cache == nil {
		panic("randomx: failed to allocate cache (check huge-pages / flags)")
	}
	C.randomx_init_cache(h.cache, unsafe.Pointer(&key[0]), C.size_t(len(key)))
	h.vm = C.randomx_create_vm(h.flags, h.cache, nil)
	if h.vm == nil {
		panic("randomx: failed to create VM")
	}
	h.key = append(h.key[:0], key...)
}

// Close releases the cache and VM. Safe to call more than once.
func (h *rxHasher) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.release()
	h.key = nil
	return nil
}

// release frees the C allocations. Must be called with h.mu held.
func (h *rxHasher) release() {
	if h.vm != nil {
		C.randomx_destroy_vm(h.vm)
		h.vm = nil
	}
	if h.cache != nil {
		C.randomx_release_cache(h.cache)
		h.cache = nil
	}
}

// -----------------------------------------------------------------------------
// Fast mode (full dataset, shared across mining threads)
// -----------------------------------------------------------------------------

// The dataset is large (~2.3 GiB) and read-only while hashing, so a single
// instance is shared process-wide by every mining VM. It is rebuilt only when
// the seed (epoch) changes. This is safe because mining is sequential per block:
// no VM is mid-hash against the old dataset when a rebuild happens, and every VM
// re-points to the new dataset (via randomx_vm_set_dataset) before its next hash.
var (
	dsMu      sync.Mutex
	dsDataset *C.randomx_dataset
	dsKey     []byte
)

// sharedDataset returns the process-wide dataset for key, building it (and
// freeing any previous one) on a key change. baseFlags must NOT include
// RANDOMX_FLAG_FULL_MEM.
func sharedDataset(baseFlags C.randomx_flags, key []byte) *C.randomx_dataset {
	dsMu.Lock()
	defer dsMu.Unlock()

	if dsDataset != nil && bytes.Equal(dsKey, key) {
		return dsDataset
	}
	if dsDataset != nil {
		C.randomx_release_dataset(dsDataset)
		dsDataset = nil
	}
	// Build a cache for the key, expand it into the dataset, then free the cache.
	cache := C.randomx_alloc_cache(baseFlags)
	if cache == nil {
		panic("randomx: failed to allocate cache for dataset")
	}
	C.randomx_init_cache(cache, unsafe.Pointer(&key[0]), C.size_t(len(key)))

	dataset := C.randomx_alloc_dataset(baseFlags)
	if dataset == nil {
		C.randomx_release_cache(cache)
		panic("randomx: failed to allocate dataset (full-mem mode needs ~2.3 GiB free RAM)")
	}
	count := uint64(C.randomx_dataset_item_count())

	// Initialise the dataset in parallel across all CPUs.
	workers := runtime.NumCPU()
	if workers < 1 {
		workers = 1
	}
	perWorker := count / uint64(workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		start := uint64(w) * perWorker
		n := perWorker
		if w == workers-1 {
			n = count - start // last worker absorbs the remainder
		}
		wg.Add(1)
		go func(start, n uint64) {
			defer wg.Done()
			C.randomx_init_dataset(dataset, cache, C.ulong(start), C.ulong(n))
		}(start, n)
	}
	wg.Wait()

	C.randomx_release_cache(cache)
	dsDataset = dataset
	dsKey = append(dsKey[:0], key...)
	return dataset
}

// rxDatasetHasher is a full-memory RandomX hasher. Its VM reads from the shared
// dataset; when the key changes it triggers a (shared) dataset rebuild and
// re-points its VM at the new dataset.
type rxDatasetHasher struct {
	flags C.randomx_flags // includes RANDOMX_FLAG_FULL_MEM (for the VM)
	base  C.randomx_flags // without FULL_MEM (for cache/dataset allocation)
	vm    *C.randomx_vm
	key   []byte
}

// Hash returns the RandomX hash of input under key, using the shared full dataset.
func (h *rxDatasetHasher) Hash(key, input []byte) common.Hash {
	if h.vm == nil || !bytes.Equal(h.key, key) {
		ds := sharedDataset(h.base, key)
		if h.vm == nil {
			h.vm = C.randomx_create_vm(h.flags, nil, ds)
			if h.vm == nil {
				panic("randomx: failed to create full-mem VM")
			}
		} else {
			C.randomx_vm_set_dataset(h.vm, ds)
		}
		h.key = append(h.key[:0], key...)
	}
	var out [32]byte
	C.randomx_calculate_hash(
		h.vm,
		unsafe.Pointer(&input[0]), C.size_t(len(input)),
		unsafe.Pointer(&out[0]),
	)
	return common.Hash(out)
}

// Close destroys the VM. The shared dataset is intentionally not freed here, as
// it may still be used by other mining VMs; it is released on the next rebuild
// or at process exit.
func (h *rxDatasetHasher) Close() error {
	if h.vm != nil {
		C.randomx_destroy_vm(h.vm)
		h.vm = nil
	}
	h.key = nil
	return nil
}
