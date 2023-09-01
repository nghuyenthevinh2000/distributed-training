package main

import (
	"sync"
	"time"
)

// thread safe float64 for Raft
type SafeFloat64 struct {
	lock  sync.RWMutex
	value float64
}

func newSafeFloat64(value float64) *SafeFloat64 {
	return &SafeFloat64{
		value: value,
	}
}

func (f *SafeFloat64) set(value float64) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.value = value
}

func (f *SafeFloat64) increment() {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.value++
}

func (f *SafeFloat64) get() float64 {
	f.lock.RLock()
	defer f.lock.RUnlock()

	return f.value
}

// thread safe State for Raft
type SafeState struct {
	lock  sync.RWMutex
	value State
}

func newSafeState(value State) *SafeState {
	return &SafeState{
		value: value,
	}
}

func (s *SafeState) set(value State) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.value = value
}

func (s *SafeState) get() State {
	s.lock.RLock()
	defer s.lock.RUnlock()

	return s.value
}

// thread safe string for Raft
type SafeString struct {
	lock  sync.RWMutex
	value string
}

func newSafeString(value string) *SafeString {
	return &SafeString{
		value: value,
	}
}

func (s *SafeString) set(value string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.value = value
}

func (s *SafeString) get() string {
	s.lock.RLock()
	defer s.lock.RUnlock()

	return s.value
}

// thread safe Time for Raft
type SafeTime struct {
	lock  sync.RWMutex
	value time.Time
}

func newSafeTime(value time.Time) *SafeTime {
	return &SafeTime{
		value: value,
	}
}

func (t *SafeTime) set(value time.Time) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.value = value
}

func (t *SafeTime) get() time.Time {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return t.value
}

func (t *SafeTime) before(b time.Time) bool {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return t.value.Before(b)
}

// thread safe index for Raft
type SafeIndex struct {
	lock sync.RWMutex
	kv   map[string]int
}

func newSafeIndex() *SafeIndex {
	return &SafeIndex{
		kv: make(map[string]int),
	}
}

func (i *SafeIndex) set(key string, value int) {
	i.lock.Lock()
	defer i.lock.Unlock()

	i.kv[key] = value
}

func (i *SafeIndex) get(key string) int {
	i.lock.RLock()
	defer i.lock.RUnlock()

	return i.kv[key]
}

func (i *SafeIndex) len() int {
	i.lock.RLock()
	defer i.lock.RUnlock()

	return len(i.kv)
}

func (i *SafeIndex) keys() []int {
	i.lock.RLock()
	defer i.lock.RUnlock()

	keys := make([]int, len(i.kv))
	for _, vals := range i.kv {
		keys = append(keys, vals)
	}

	return keys
}
