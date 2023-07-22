package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

var (
	// this lock protects os.Stderr resource
	logLock sync.Mutex
	// this lock protects os.Stdout resource
	sendLock sync.Mutex
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

type Task struct {
	duration time.Duration
	fn       func()
}

type Node struct {
	nodeId  string
	nodeIds []string

	handler        map[string]func(req Request) error
	callbacks      map[float64]func(req Request) error
	messages       map[float64]bool
	periodic_tasks []Task
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
	server := CounterServer{}
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

func (node *Node) every(duration time.Duration, block func()) {
	node.periodic_tasks = append(node.periodic_tasks, Task{duration, block})
}

func (node *Node) startPeriodicTasks() {
	for _, task := range node.periodic_tasks {
		go func(task Task) {
			for {
				task.fn()
				time.Sleep(task.duration)
			}
		}(task)
	}
}

func (node *Node) onInit(req Request) error {
	reqBody := req.Body.(map[string]interface{})
	node.nodeId = reqBody["node_id"].(string)
	node.nodeIds = convertArrInterfaceToArrString(reqBody["node_ids"].([]interface{}))

	reqBody["type"] = "init_ok"
	node.reply(req, reqBody)
	logSafe(fmt.Sprintf("Node %s initialized", node.nodeId))

	node.startPeriodicTasks()
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

func logSafe(msg string) {
	logLock.Lock()
	defer logLock.Unlock()

	fmt.Fprintln(os.Stderr, msg)
}

func (node *Node) reply(req Request, reqBody map[string]interface{}) {
	body := req.Body.(map[string]interface{})

	reqBody["in_reply_to"] = body["msg_id"]

	node.sendSafe(req.Src, reqBody)
}
