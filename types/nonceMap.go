package types

import (
	"sync"

	"github.com/filecoin-project/go-address"
)

type NonceMap struct {
	nonceMap map[address.Address]uint64
	lk       sync.RWMutex
}

func NewNonceMap() *NonceMap {
	return &NonceMap{
		nonceMap: make(map[address.Address]uint64),
		lk:       sync.RWMutex{},
	}
}

func (nonceMap *NonceMap) Get(addr address.Address) (uint64, bool) {
	nonceMap.lk.RLock()
	defer nonceMap.lk.RUnlock()
	if val, ok := nonceMap.nonceMap[addr]; ok {
		return val, ok
	}
	return 0, false
}

func (nonceMap *NonceMap) Add(addr address.Address, val uint64) {
	nonceMap.lk.Lock()
	defer nonceMap.lk.Unlock()
	nonceMap.nonceMap[addr] = val
}

func (nonceMap *NonceMap) Len() int {
	nonceMap.lk.RLock()
	defer nonceMap.lk.RUnlock()
	return len(nonceMap.nonceMap)
}

func (nonceMap *NonceMap) Each(f func(addr address.Address, val uint64)) {
	nonceMap.lk.RLock()
	defer nonceMap.lk.RUnlock()
	for addr, value := range nonceMap.nonceMap {
		f(addr, value)
	}
}
