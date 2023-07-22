package main

import (
	"fmt"
	"math"
	"sync"
	"time"
)

var (
	counter_lock sync.RWMutex
)

type CounterServer struct {
	node     *Node
	gcounter *PNCounter
}

func (g *CounterServer) init() {
	g.node = newNode()
	g.gcounter = &PNCounter{}
	g.gcounter.init()

	g.node.on("read", func(req Request) error {
		reqBody := req.Body.(map[string]interface{})
		reqBody["type"] = "read_ok"
		reqBody["value"] = g.gcounter.read()

		logSafe(fmt.Sprintf("Read value %v", reqBody["value"]))

		g.node.reply(req, reqBody)

		return nil
	})

	g.node.on("add", func(req Request) error {
		reqBody := req.Body.(map[string]interface{})
		delta := reqBody["delta"].(float64)
		g.gcounter.add(g.node.nodeId, delta)

		reqBody["type"] = "add_ok"
		delete(reqBody, "delta")

		g.node.reply(req, reqBody)

		return nil
	})

	g.node.on("replicate", func(req Request) error {
		reqBody := req.Body.(map[string]interface{})
		elements := reqBody["value"]
		g.gcounter.merge(elements)

		return nil
	})

	g.node.every(5*time.Second, func() {
		values := g.gcounter.to_json()
		logSafe(fmt.Sprintf("Replicating current counter map %v", values))

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

type GCounter struct {
	counter map[string]float64
}

func (g *GCounter) init() {
	g.counter = make(map[string]float64)
}

func (g *GCounter) to_json() map[string]float64 {
	return g.counter
}

func (g *GCounter) read() float64 {
	counter_lock.RLock()
	var sum float64
	for _, value := range g.counter {
		sum += value
	}
	counter_lock.RUnlock()

	return sum
}

func (g *GCounter) merge(other interface{}) {
	parsed := other.(map[string]interface{})
	counter_lock.Lock()
	for node_id, value := range parsed {
		if _, ok := g.counter[node_id]; !ok {
			g.counter[node_id] = 0
		}
		g.counter[node_id] = math.Max(g.counter[node_id], value.(float64))
	}
	counter_lock.Unlock()
}

func (g *GCounter) add(node_id string, element float64) {
	counter_lock.Lock()
	if _, ok := g.counter[node_id]; !ok {
		g.counter[node_id] = 0
	}

	g.counter[node_id] += element
	counter_lock.Unlock()
}

type PNCounter struct {
	inc *GCounter
	dec *GCounter
}

func (p *PNCounter) init() {
	p.inc = &GCounter{}
	p.inc.init()
	p.dec = &GCounter{}
	p.dec.init()
}

func (p *PNCounter) to_json() map[string]map[string]float64 {
	return map[string]map[string]float64{
		"inc": p.inc.to_json(),
		"dec": p.dec.to_json(),
	}
}

func (p *PNCounter) read() float64 {
	value := p.inc.read() - p.dec.read()
	return value
}

func (p *PNCounter) merge(other interface{}) {
	parsed := other.(map[string]interface{})

	p.inc.merge(parsed["inc"])
	p.dec.merge(parsed["dec"])
}

func (p *PNCounter) add(node_id string, delta float64) {
	if delta >= 0 {
		p.inc.add(node_id, delta)
	} else {
		p.dec.add(node_id, -delta)
	}
}
