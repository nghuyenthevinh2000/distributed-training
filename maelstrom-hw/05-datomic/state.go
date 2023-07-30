package main

import "fmt"

const KEY = "root"

type State struct {
	node *Node
}

func (s *State) init(node *Node) {
	s.node = node
}

func (s *State) transact(txs []interface{}) []interface{} {
	retry := 0

	for retry < 3 {
		// retrieve map
		reqBody := map[string]interface{}{
			"type": "read",
			"key":  KEY,
		}
		res := s.node.sync_rpc("lin-kv", reqBody)
		resBody := res.Body.(map[string]interface{})
		logSafe(fmt.Sprintf("KEY resBody: %+v", resBody))
		m := &Map{}
		// initialize map with value populated
		if resBody["type"].(string) == "read_ok" {
			m.init(s.node, resBody["value"].(string), true, []interface{}{})
			// get the list of [k - mapping id]
			value := m.value()
			logSafe(fmt.Sprintf("id mapping value: %+v", value))
			// populate map kv
			m.from_json(value)
			logSafe(fmt.Sprintf("initial m: %+v", m.to_json()))
		} else {
			m.init(s.node, s.node.newId(), false, []interface{}{})
		}

		// apply txs on map
		new_m, res_txs := m.transact(txs)
		logSafe(fmt.Sprintf("old_m: %+v, new_m: %+v, res_txs: %+v", m.to_json(), new_m.to_json(), res_txs))

		// if map has no change
		if m.m_thunk.id == new_m.m_thunk.id {
			return res_txs
		}

		// if map has changed, save new map
		new_m.save()

		// update mapping id of map
		reqBody = map[string]interface{}{
			"type":                 "cas",
			"key":                  KEY,
			"from":                 m.m_thunk.id,
			"to":                   new_m.m_thunk.id,
			"create_if_not_exists": true,
		}
		res = s.node.sync_rpc("lin-kv", reqBody)
		resBody = res.Body.(map[string]interface{})
		if resBody["type"].(string) == "cas_ok" {
			logSafe("append map successfully")
			return res_txs
		}

		if resBody["code"].(float64) == 22 {
			logSafe(fmt.Sprintf("append map failed: %+v, current value: %+v, to value: %+v", resBody["text"], m.m_thunk.id, new_m.m_thunk.id))
			retry += 1
		}
	}

	return []interface{}{}
}
