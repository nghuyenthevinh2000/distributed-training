package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

var (
	// this lock protects os.Stderr resource
	logLock sync.Mutex
	// this lock protects os.Stdout resource
	sendLock sync.Mutex
	// this lock protects callbacks resource
	callbackLock sync.RWMutex
	// this lock protects Id resource
	idLock sync.RWMutex

	// id
	id int64
)

type Request struct {
	Id   int         `json:"id"`
	Src  string      `json:"src"`
	Dest string      `json:"dest"`
	Body interface{} `json:"body"`
}

type Response struct {
	Src  string      `json:"src"`
	Dest string      `json:"dest"`
	Body interface{} `json:"body"`
}

type Node struct {
	nodeId    string
	nodeIds   []string
	nextMsgId float64

	handler   map[string]func(req Request) error
	callbacks map[float64]func(req Request) error
	messages  map[float64]bool
}

func newNode() *Node {
	node := &Node{}

	node.handler = make(map[string]func(req Request) error)
	node.messages = make(map[float64]bool)
	node.callbacks = make(map[float64]func(req Request) error)
	node.on("init", node.onInit)

	return node
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)

	// create a server instance
	server := Transactor{}
	server.init()

	for scanner.Scan() {
		line := scanner.Text()
		logSafe(fmt.Sprintf("Received %s", line))

		req := Request{}
		err := json.Unmarshal([]byte(line), &req)
		if err != nil {
			logSafe(fmt.Sprintf("Error unmarshaling request for line %s: %s", line, err))
			continue
		}

		server.node.handle(req)
	}
}

func (node *Node) handle(req Request) {
	reqBody := req.Body.(map[string]interface{})
	var handlerFunc func(req Request) error

	if _, ok := reqBody["in_reply_to"]; ok {
		callbackLock.RLock()
		if _, ok := node.callbacks[reqBody["in_reply_to"].(float64)]; ok {
			handlerFunc = node.callbacks[reqBody["in_reply_to"].(float64)]
		}
		callbackLock.RUnlock()
	}

	if _, ok := node.handler[reqBody["type"].(string)]; ok {
		handlerFunc = node.handler[reqBody["type"].(string)]
	}

	if handlerFunc == nil {
		logSafe(fmt.Sprintf("No handler for %v", req))
		return
	}

	// execute handler
	go func() {
		if err := handlerFunc(req); err != nil {
			logSafe(fmt.Sprintf("Error executing handler for %v: %s", req, err))
		}
	}()
}

// register a new message type handler
func (node *Node) on(msgType string, msgHandler func(req Request) error) {
	if _, ok := node.handler[msgType]; ok {
		panic(fmt.Sprintf("Already have a handler for %s!", msgType))
	}

	node.handler[msgType] = msgHandler
}

func (node *Node) onInit(req Request) error {
	reqBody := req.Body.(map[string]interface{})
	node.nodeId = reqBody["node_id"].(string)
	node.nodeIds = convertArrInterfaceToArrString(reqBody["node_ids"].([]interface{}))

	reqBody["type"] = "init_ok"
	node.reply(req, reqBody)
	logSafe(fmt.Sprintf("Node %s initialized", node.nodeId))
	return nil
}

func convertArrInterfaceToArrString(arr []interface{}) []string {
	newArr := make([]string, len(arr))
	for i, v := range arr {
		newArr[i] = v.(string)
	}

	return newArr
}

func (node *Node) sendSafe(dest string, body interface{}) {
	res := Response{
		Src:  node.nodeId,
		Dest: dest,
		Body: body,
	}

	// send safe
	jsonBytes, err := json.Marshal(res)
	if err != nil {
		logSafe(fmt.Sprintf("Error marshaling response: %s for %v", err, res))
		return
	}

	sendLock.Lock()
	defer sendLock.Unlock()
	fmt.Fprintln(os.Stdout, string(jsonBytes))
}

func (node *Node) rpc(dest string, reqBody map[string]interface{}, callbackHandler func(req Request) error) {
	callbackLock.Lock()
	node.nextMsgId++
	msgId := node.nextMsgId
	node.callbacks[msgId] = callbackHandler
	reqBody["msg_id"] = msgId
	callbackLock.Unlock()

	node.sendSafe(dest, reqBody)
}

func (node *Node) sync_rpc(dest string, reqBody map[string]interface{}) Request {
	p := newPromise()

	// send rpc
	node.rpc(dest, reqBody, func(req Request) error {
		// resolve promise
		p.resolve(req)
		return nil
	})

	// start waiting for a response
	return p.await()
}

func (node *Node) reply(req Request, reqBody map[string]interface{}) {
	body := req.Body.(map[string]interface{})

	reqBody["in_reply_to"] = body["msg_id"]

	node.sendSafe(req.Src, reqBody)
}

func logSafe(msg string) {
	logLock.Lock()
	defer logLock.Unlock()

	fmt.Fprintln(os.Stderr, msg)
}

type RPCError struct {
	Code int
	Text string
}

var ErrorCodes = map[int]string{
	0:  "timeout",
	10: "not supported",
	11: "temporarily unavailable",
	12: "malformed request",
	13: "crash",
	14: "abort",
	20: "key does not exist",
	22: "precondition failed",
	30: "txn conflict",
	32: "Unable to save thunk",
	34: "Obsolete map",
}

func newRPCError(code int) RPCError {
	return RPCError{
		Code: code,
		Text: ErrorCodes[code],
	}
}

func (e RPCError) LogError(value string) {
	logSafe(fmt.Sprintf("error %v with value: %s", e.Text, value))
}

func (node *Node) newId() string {
	idLock.Lock()
	id += 1
	idLock.Unlock()

	return fmt.Sprintf("%s-%d", node.nodeId, id)
}

func (node *Node) getId() int64 {
	idLock.RLock()
	defer idLock.RUnlock()

	return id
}
