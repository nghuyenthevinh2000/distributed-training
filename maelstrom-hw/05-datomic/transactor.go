package main

import (
	"sync"
)

var (
	lock sync.RWMutex
)

type Transactor struct {
	node  *Node
	state State
}

func (t *Transactor) init() {
	t.node = newNode()
	t.state = State{}
	t.state.init(t.node)

	t.node.on("txn", func(req Request) error {
		reqBody := req.Body.(map[string]interface{})
		txs := reqBody["txn"].([]interface{})

		lock.Lock()
		res_txs := t.state.transact(txs)
		lock.Unlock()

		reqBody["type"] = "txn_ok"
		reqBody["txn"] = res_txs

		t.node.reply(req, reqBody)
		return nil
	})
}
