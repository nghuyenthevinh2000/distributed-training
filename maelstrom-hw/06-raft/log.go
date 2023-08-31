package main

import (
	"fmt"
	"sync"
)

var log_lock sync.RWMutex

type Entry struct {
	term float64
	op   map[string]interface{}
	src  string
}

type Log struct {
	node    *Node
	entries []Entry
}

func (l *Log) init(node *Node) {
	l.node = node
	l.entries = []Entry{
		{term: 0, op: nil, src: ""},
	}
}

func (l *Log) getEntry(index int) Entry {
	log_lock.RLock()
	defer log_lock.RUnlock()

	if index > len(l.entries) {
		return Entry{}
	}

	return l.entries[index-1]
}

func (l *Log) appendEntry(entries []Entry) {
	log_lock.Lock()
	defer log_lock.Unlock()

	l.entries = append(l.entries, entries...)

	logSafe(fmt.Sprintf("new entries: %v", l.entries))
}

func (l *Log) truncate(len int) {
	log_lock.Lock()
	defer log_lock.Unlock()

	l.entries = l.entries[len:]
}

func (l *Log) lastEntry() Entry {
	log_lock.RLock()
	defer log_lock.RUnlock()

	return l.entries[len(l.entries)-1]
}

func (l *Log) size() int {
	log_lock.RLock()
	defer log_lock.RUnlock()

	return len(l.entries)
}

func (l *Log) fromIndex(index int) []Entry {
	log_lock.RLock()
	defer log_lock.RUnlock()

	if index <= 0 {
		newRPCError(39).LogError("index must be greater than 0")
		return nil
	}

	return l.entries[index-1:]
}
