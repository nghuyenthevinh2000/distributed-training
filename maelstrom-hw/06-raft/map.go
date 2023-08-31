package main

import (
	"fmt"
	"sync"
)

var map_lock sync.RWMutex

type Map struct {
	kv map[float64]interface{}
}

func (m *Map) init() {
	m.kv = make(map[float64]interface{})
}

func (m *Map) apply(op map[string]interface{}) (map[string]interface{}, error) {
	logSafe(fmt.Sprintf("trying to apply op: %v", op))

	k := op["key"].(float64)

	switch op["type"] {
	case "read":
		map_lock.RLock()
		defer map_lock.RUnlock()

		if v, ok := m.kv[k]; ok {
			res := map[string]interface{}{}
			res["type"] = "read_ok"
			res["value"] = v

			return res, nil
		} else {
			err := newRPCError(20)
			err.LogError(fmt.Sprintf("key %f does not exist in map", k))

			return nil, err.Error("")
		}
	case "write":
		map_lock.Lock()
		defer map_lock.Unlock()

		m.kv[k] = op["value"]
		res := map[string]interface{}{}
		res["type"] = "write_ok"

		return res, nil
	case "cas":
		map_lock.Lock()
		defer map_lock.Unlock()

		if v, ok := m.kv[k]; ok {
			if v == op["from"] {

				m.kv[k] = op["to"]
				res := map[string]interface{}{}
				res["type"] = "cas_ok"

				return res, nil
			} else {
				err := newRPCError(35)
				err.LogError(fmt.Sprintf("expected %f, but had %f", op["from"].(float64), m.kv[k].(float64)))

				return nil, err.Error("")
			}
		} else {
			err := newRPCError(20)
			err.LogError(fmt.Sprintf("key %f does not exist in map", k))

			return nil, err.Error("")
		}
	}

	return nil, nil
}
