package main

import (
	"fmt"
	"strconv"
	"time"
)

const (
	KEY           = "root"
	SVC           = "lww-kv"
	FALLBACK_TIME = 200 * time.Millisecond
)

// keep track of latest timestamp
var newest_timestamp string

type State struct {
	node *Node
}

func (s *State) init(node *Node) {
	s.node = node
}

func (s *State) transact(txs []interface{}) []interface{} {
	// retrieve map
	m := s.read_map()

	// apply txs on map
	new_m, res_txs := m.transact(txs)
	logSafe(fmt.Sprintf("old_m: %+v, new_m: %+v, res_txs: %+v", m.to_json(), new_m.to_json(), res_txs))

	// update map
	s.update_map(m, &new_m)
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
	read_retry := 0
	for read_retry < 5 {
		res := s.node.sync_rpc(SVC, reqBody)
		resBody := res.Body.(map[string]interface{})
		logSafe(fmt.Sprintf("KEY resBody: %+v", resBody))

		// initialize map with value populated
		if resBody["type"].(string) == "read_ok" {
			id, time_stamp := resBody["value"].([]interface{})[0].(string), resBody["value"].([]interface{})[1].(string)
			a, _ := strconv.ParseInt(time_stamp, 10, 64)
			b, _ := strconv.ParseInt(newest_timestamp, 10, 64)

			// check time stamp
			logSafe(fmt.Sprintf("reading root with old timestamp: %s, new timestamp: %s", time_stamp, newest_timestamp))
			if a < b {
				newRPCError(34).LogError(fmt.Sprintf("failed id %s", id))
				read_retry++
				time.Sleep(FALLBACK_TIME)
				continue
			}

			newest_timestamp = time_stamp

			m.init(s.node, id, true, []interface{}{})
			// get the list of [k - mapping id]
			value := m.value()
			logSafe(fmt.Sprintf("id mapping value: %+v", id))
			// populate map kv
			m.from_json(value)
			logSafe(fmt.Sprintf("initial m: %+v", m.to_json()))

			break
		}

		// handling errors
		if resBody["code"].(float64) == 20 {
			newRPCError(20).LogError("reading root failed")
			read_retry++
			time.Sleep(FALLBACK_TIME)
		}
	}

	// in the event that have tried 3 failed times
	if read_retry == 5 {
		new_id := s.node.newId()
		logSafe(fmt.Sprintf("can't find mapping id, suggesting new one: %s", new_id))
		m.init(s.node, new_id, false, []interface{}{})
	}

	return m
}

// update map function
// true: update map successfully
// false: fail to update map
func (s *State) update_map(old_map, new_map *Map) bool {
	// if map has no change
	if old_map.m_thunk.id == new_map.m_thunk.id {
		return false
	}

	// if map has changed, save new map
	new_map.save()

	// update mapping id of map
	logSafe("UPDATING MAPPING ID OF MAP")

	current_time_stamp := time.Now().UnixNano()
	current_time_stamp_str := strconv.FormatInt(current_time_stamp, 10)
	newest_timestamp = current_time_stamp_str

	reqBody := map[string]interface{}{
		"type":  "write",
		"key":   KEY,
		"value": []interface{}{new_map.m_thunk.id, current_time_stamp_str},
	}

	res := s.node.sync_rpc(SVC, reqBody)
	resBody := res.Body.(map[string]interface{})
	if resBody["type"].(string) == "write_ok" {
		logSafe("update map successfully")
		return true
	}

	// this is just in case that a lww-kv node that handles this transaction is not up - to - date
	if resBody["code"].(float64) == 20 {
		newRPCError(20).LogError(fmt.Sprintf("update map failed: key = %s", new_map.m_thunk.id))
		return false
	}

	if resBody["code"].(float64) == 22 {
		logSafe(fmt.Sprintf("update map failed: %+v, current value: %+v, to value: %+v", resBody["text"], old_map.m_thunk.id, new_map.m_thunk.id))
		return false
	}

	return false
}
