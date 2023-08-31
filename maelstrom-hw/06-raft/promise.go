package main

import "time"

const (
	TIMEOUT = 5 * time.Second
)

type Promise struct {
	request chan Request
}

func newPromise() *Promise {
	p := &Promise{}
	p.request = make(chan Request, 1)
	return p
}

func (p *Promise) await() Request {
	select {
	case res := <-p.request:
		return res
	case <-time.After(TIMEOUT):
		return Request{}
	}
}

func (p *Promise) resolve(req Request) {
	p.request <- req
}
