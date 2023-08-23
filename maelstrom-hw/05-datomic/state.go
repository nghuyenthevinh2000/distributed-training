package main

import (
	"fmt"
	"time"
)

const (
	KEY           = "root"
	SVC           = "lww-kv"
	FALLBACK_TIME = 300 * time.Millisecond
	RECENT_LENGTH = 6
)

type State struct {
	node *Node
	// table of three most recent values
	// this feature provides more stable recent reads
	recent []string
}

func (s *State) init(node *Node) {
	s.node = node
	s.recent = make([]string, 0)
}

// compare and set if ROOT exists
// write if it doesn't
// WRITE:
// if it writes to a lww instance not yet updated, retry write
// if it's value is outdated, retry read -> write
func (s *State) transact(txs []interface{}) []interface{} {

	logSafe("TRANSACTION STARTED")

	var res_txs []interface{}
	var new_m Map

	for true {
		// retrieve map
		m := s.read_map()

		// apply txs on map
		new_m, res_txs = m.transact(txs)
		logSafe(fmt.Sprintf("old_m: %+v, new_m: %+v, res_txs: %+v", m.to_json(), new_m.to_json(), res_txs))

		// update map
		success := s.update_map(m, &new_m)

		if success {
			break
		} else {
			time.Sleep(FALLBACK_TIME)
		}
	}

	return res_txs
}

// how to ensure that map will always get the newest value from 5 nodes in lww-kv
func (s *State) read_map() *Map {
	logSafe("READING MAP")

	reqBody := map[string]interface{}{
		"type": "read",
		"key":  KEY,
	}

	m := &Map{}

	// if newly created node, register map to root
	if s.node.getId() == 0 && s.node.nodeId == "n0" {
		new_id := s.node.newId()
		logSafe(fmt.Sprintf("newly initialized, suggesting new id: %s", new_id))

		// ROOT doesn't exist, write to SVC
		reqBody := map[string]interface{}{
			"type":  "write",
			"key":   KEY,
			"value": new_id,
		}

		res := s.node.sync_rpc(SVC, reqBody)
		resBody := res.Body.(map[string]interface{})
		if resBody["type"].(string) == "write_ok" {
			logSafe("register map successfully")
		}

		m.init(s.node, new_id, false, []interface{}{})
		// add empty map to new_id
		m.save()

		return m
	}

	// keep reading until success
	for true {
		res := s.node.sync_rpc(SVC, reqBody)
		resBody := res.Body.(map[string]interface{})
		logSafe(fmt.Sprintf("KEY resBody: %+v", resBody))

		// initialize map with value populated
		if resBody["type"].(string) == "read_ok" {
			id := resBody["value"].(string)

			// check if id is in recent[!0]
			recent := false
			logSafe(fmt.Sprintf("recent: %+v", s.recent))
			for i := 1; i < len(s.recent); i++ {
				if s.recent[i] == id {
					logSafe(fmt.Sprintf("id = %s, in recent[%d]", id, i))
					recent = true
					time.Sleep(FALLBACK_TIME)
					break
				}
			}
			if recent {
				continue
			}

			m.init(s.node, id, true, []interface{}{})

			// get the list of [k - mapping id], need retry
			value := m.value()
			logSafe(fmt.Sprintf("id = %s, with mapping value: %+v", id, value))
			if value == nil {
				newRPCError(20).LogError(fmt.Sprintf("thunk failed to read id: %s", id))
				time.Sleep(FALLBACK_TIME)
				continue
			}

			// populate map kv
			m.from_json(value)
			logSafe(fmt.Sprintf("initial m: %+v", m.to_json()))

			break
		}

		// handling errors
		if resBody["code"].(float64) == 20 {
			newRPCError(20).LogError("reading root failed")
		}
	}

	return m
}

// update map function
// true: update map successfully
// false: fail to update map
func (s *State) update_map(old_map, new_map *Map) bool {
	// if map has no change
	if old_map.m_thunk.id == new_map.m_thunk.id {
		return true
	}

	// if map has changed, save new map
	new_map.save()

	// update mapping id of map
	logSafe("UPDATING MAPPING ID OF MAP")

	reqBody := map[string]interface{}{
		"type": "cas",
		"key":  KEY,
		"from": old_map.m_thunk.id,
		"to":   new_map.m_thunk.id,
	}

	for true {
		res := s.node.sync_rpc(SVC, reqBody)
		resBody := res.Body.(map[string]interface{})
		logSafe(fmt.Sprintf("update map resBody: %+v", resBody))
		if resBody["type"].(string) == "cas_ok" {
			logSafe("update map successfully")

			// update to recent[0]
			s.recent_prepend(new_map.m_thunk.id)

			return true
		}

		// this is just in case that a lww-kv instance that handles this transaction is not up - to - date
		if resBody["code"].(float64) == 20 {
			newRPCError(20).LogError(fmt.Sprintf("update map root failed: value = %s", new_map.m_thunk.id))
			time.Sleep(FALLBACK_TIME)
			continue
		}

		// in this scenario, there is  a mismatch between the current value of the map and the value that we want to update to
		// a merge between current value stored on lww-kv and the value that we want to update to is needed
		if resBody["code"].(float64) == 22 {
			logSafe(fmt.Sprintf("update map root failed: %+v, current value: %+v, to value: %+v", resBody["text"], old_map.m_thunk.id, new_map.m_thunk.id))
			return false
		}
	}

	return false
}

// perform queue push
func (s *State) recent_prepend(id string) {
	// if recent is full, remove last item
	if len(s.recent) == RECENT_LENGTH {
		s.recent = s.recent[:RECENT_LENGTH-1]
	}

	s.recent = append([]string{id}, s.recent...)
}

// perform merge
func (s *State) merge() {

}
