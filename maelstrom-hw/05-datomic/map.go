package main

import (
	"fmt"
	"sort"
)

type Map struct {
	kv map[float64]*Thunk
	// a map is also a thunk
	m_thunk *Thunk
}

func (m *Map) value() []interface{} {
	return m.m_thunk.getValue()
}

func (m *Map) from_json(json []interface{}) {
	for _, pair := range json {
		// populate kv
		arr := pair.([]interface{})
		k := arr[0].(float64)
		id := arr[1].(string)
		m.kv[k] = newThunk(m.m_thunk.node, id, []interface{}{}, true)
	}

	logSafe(fmt.Sprintf("from_json kv: %+v, mapping: %+v", m.kv, m.m_thunk.value))
}

func (m *Map) to_json() []interface{} {
	json := []interface{}{}
	json = append(json, m.m_thunk.id, m.m_thunk.value)

	return json
}

func (m *Map) init(node *Node, id string, saved bool, value []interface{}) {
	m.kv = make(map[float64]*Thunk)
	m.m_thunk = newThunk(node, id, value, saved)
}

// save map on store
func (m *Map) save() {
	// save id mapping of map
	logSafe("saving map")

	// save id mapping of each thunk
	m_thunk_key := []float64{}
	m_thunk_arr := []interface{}{}

	for k, v := range m.kv {
		thunk := v
		thunk.save()

		// deflate kv into array for saving id mapping
		m_thunk_key = append(m_thunk_key, k)
	}

	// sort id mapping
	sort.Float64s(m_thunk_key)

	// construct sorted m_thunk_arr
	for _, k := range m_thunk_key {
		thunk := m.kv[k]
		m_thunk_arr = append(m_thunk_arr, []interface{}{k, thunk.id})
	}

	m.m_thunk.value = m_thunk_arr
	m.m_thunk.save()
}

func clone(m Map) Map {
	new_m := Map{}
	new_m.init(m.m_thunk.node, m.m_thunk.id, false, []interface{}{})

	// copying value
	new_m.m_thunk.value = make([]interface{}, len(m.m_thunk.value))
	copy(new_m.m_thunk.value, m.m_thunk.value)

	// copying kv
	for k, v := range m.kv {
		old_thunk := v
		new_value := make([]interface{}, len(old_thunk.value))
		copy(new_value, old_thunk.value)
		new_m.kv[k] = newThunk(m.m_thunk.node, old_thunk.id, new_value, old_thunk.saved)
	}
	return new_m
}

func (m Map) transact(txs []interface{}) (Map, []interface{}) {
	res_txs := []interface{}{}
	new_map := clone(m)

	for _, i := range txs {
		tx := i.([]interface{})
		f := tx[0].(string)
		k := tx[1].(float64)
		v := tx[2]

		switch f {
		case "r":
			var thunk *Thunk
			if _, ok := new_map.kv[k]; ok {
				thunk = new_map.kv[k]
			} else {
				thunk = newThunk(m.m_thunk.node, m.m_thunk.node.newId(), []interface{}{}, false)
			}

			res_txs = append(res_txs, []interface{}{"r", k, thunk.getValue()})
		case "append":
			var thunk *Thunk

			logSafe("append")
			new_map.m_thunk.id = new_map.m_thunk.node.newId()
			if _, ok := new_map.kv[k]; ok {
				old_thunk := new_map.kv[k]
				thunk = newThunk(m.m_thunk.node, m.m_thunk.node.newId(), old_thunk.getValue(), false)
			} else {
				thunk = newThunk(m.m_thunk.node, m.m_thunk.node.newId(), []interface{}{}, false)
			}

			thunk.value = append(thunk.value, v)
			new_map.kv[k] = thunk

			logSafe(fmt.Sprintf("item %f value: %+v", k, new_map.kv[k]))

			res_txs = append(res_txs, tx)
		}
	}

	return new_map, res_txs
}
