package main

import (
	"fmt"
	"sync"
	"time"
)

var (
	// this set_lock protects set resource
	set_lock sync.RWMutex
)

type GSetServer struct {
	node *Node
	gset *GSet
}

func (g *GSetServer) init() {
	g.node = newNode()
	g.gset = &GSet{}
	g.gset.init()

	g.node.on("read", func(req Request) error {
		reqBody := req.Body.(map[string]interface{})
		reqBody["type"] = "read_ok"
		reqBody["value"] = g.gset.to_json()

		g.node.reply(req, reqBody)

		return nil
	})

	g.node.on("add", func(req Request) error {
		reqBody := req.Body.(map[string]interface{})
		element := reqBody["element"].(float64)
		g.gset.add(element)

		reqBody["type"] = "add_ok"
		delete(reqBody, "element")

		g.node.reply(req, reqBody)

		return nil
	})

	g.node.on("replicate", func(req Request) error {
		reqBody := req.Body.(map[string]interface{})
		elements := reqBody["value"].([]interface{})
		g.gset.merge(elements)

		return nil
	})

	g.node.every(5*time.Second, func() {
		values := g.gset.to_json()
		logSafe(fmt.Sprintf("Replicating current set %v", values))

		for _, node_id := range g.node.nodeIds {
			if node_id == g.node.nodeId {
				continue
			}
			reqBody := make(map[string]interface{})
			reqBody["type"] = "replicate"
			reqBody["value"] = values

			g.node.sendSafe(node_id, reqBody)
		}
	})
}

type GSet struct {
	set map[float64]bool
}

func (g *GSet) init() {
	g.set = make(map[float64]bool)
}

func (g *GSet) to_json() []float64 {
	set_lock.RLock()
	values := make([]float64, 0, len(g.set))
	for k := range g.set {
		values = append(values, k)
	}
	set_lock.RUnlock()

	return values
}

func (g *GSet) merge(other []interface{}) {
	set_lock.RLock()
	for _, element := range other {
		g.set[element.(float64)] = true
	}
	set_lock.RUnlock()
}

func (g *GSet) add(element float64) {
	set_lock.RLock()
	g.set[element] = true
	set_lock.RUnlock()
}
