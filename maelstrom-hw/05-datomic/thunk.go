package main

import "fmt"

const SVC = "lin-kv"

type Thunk struct {
	node  *Node
	id    string
	value []interface{}
	saved bool
}

func newThunk(node *Node, id string, value []interface{}, saved bool) *Thunk {
	t := &Thunk{}

	t.node = node
	t.id = id
	t.value = value
	t.saved = saved

	return t
}

func (t *Thunk) getValue() []interface{} {
	res := t.node.sync_rpc(SVC, map[string]interface{}{
		"type": "read",
		"key":  t.id,
	})

	resBody := res.Body.(map[string]interface{})
	logSafe(fmt.Sprintf("getValue resBody: %+v", resBody))
	if resBody["type"].(string) == "read_ok" {
		t.value = resBody["value"].([]interface{})
	} else {
		newRPCError(20).LogError(resBody["text"].(string))
	}

	return t.value
}

func (t *Thunk) save() {
	if !t.saved {
		res := t.node.sync_rpc(SVC, map[string]interface{}{
			"type":  "write",
			"key":   t.id,
			"value": t.value,
		})

		resBody := res.Body.(map[string]interface{})
		logSafe(fmt.Sprintf("save resBody: %+v", resBody))
		if resBody["type"] == "write_ok" {
			t.saved = true
			logSafe(fmt.Sprintf("saved thunk id %s, value %+v", t.id, t.value))
		} else {
			newRPCError(32).LogError(resBody["text"].(string))
		}
	}
}
