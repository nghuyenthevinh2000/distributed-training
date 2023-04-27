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
	nodeId       string
	nodeIds      []string
	nextMsgId    float64
	handler      map[string]func(req Request) error
	callbacks    map[float64]func(req Request) error
	messages     map[float64]bool
	neighbors    []string
	logLock      sync.Mutex
	sendLock     sync.Mutex
	callbackLock sync.RWMutex
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

func init() {
	handler = make(map[string]func(req Request) error)
	messages = make(map[float64]bool)
	callbacks = make(map[float64]func(req Request) error)
	on("init", onInit)
	on("topology", onTopology)
	on("read", onRead)
	on("broadcast", onBroadcast)
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		line := scanner.Text()
		logSafe(fmt.Sprintf("Received %s", line))

		req := Request{}
		err := json.Unmarshal([]byte(line), &req)
		if err != nil {
			logSafe(fmt.Sprintf("Error unmarshaling request for line %s: %s", line, err))
			continue
		}

		reqBody := req.Body.(map[string]interface{})
		var handlerFunc func(req Request) error

		// verify input
		if _, ok := reqBody["in_reply_to"]; ok {
			callbackLock.RLock()
			if _, ok := callbacks[reqBody["in_reply_to"].(float64)]; ok {
				handlerFunc = callbacks[reqBody["in_reply_to"].(float64)]
			}
			callbackLock.RUnlock()
		}

		if handlerFunc == nil {
			if _, ok := handler[reqBody["type"].(string)]; ok {
				handlerFunc = handler[reqBody["type"].(string)]
			}
		}

		if handlerFunc == nil {
			logSafe(fmt.Sprintf("No handler for %v", req))
			continue
		}

		// execute handler
		go func() {
			if err := handlerFunc(req); err != nil {
				logSafe(fmt.Sprintf("Error executing handler for %v: %s", req, err))
			}
		}()
	}
}

// register a new message type handler
func on(msgType string, msgHandler func(req Request) error) {
	if _, ok := handler[msgType]; ok {
		panic(fmt.Sprintf("Already have a handler for %s!", msgType))
	}

	handler[msgType] = msgHandler
}

func onBroadcast(req Request) error {
	reqBody := req.Body.(map[string]interface{})
	message := reqBody["message"].(float64)

	if _, ok := reqBody["msg_id"]; ok {
		delete(reqBody, "message")
		reqBody["type"] = "broadcast_ok"
		reply(req, reqBody)
	}

	if _, ok := messages[message]; !ok {
		messages[message] = true

		unacked := make([]string, len(neighbors))
		copy(unacked, neighbors)
		unacked = removeElement(unacked, req.Src)

		for len(unacked) > 0 {
			logSafe(fmt.Sprintf("Need to replicate %f to %s", message, unacked[0]))

			for _, dest := range unacked {
				clonedBody := make(map[string]interface{})
				clonedBody["type"] = "broadcast"
				clonedBody["message"] = message
				rpc(dest, clonedBody, func(req Request) error {
					logSafe(fmt.Sprintf("Received ack for %f from %s", message, req.Src))

					reqBody := req.Body.(map[string]interface{})
					if reqBody["type"] == "broadcast_ok" {
						unacked = removeElement(unacked, req.Src)
					}

					return nil
				})
			}

			// sleep for 1 second
			time.Sleep(1 * time.Second)
		}

		logSafe(fmt.Sprintf("Done with message %f", message))
	}

	return nil
}

func onRead(req Request) error {
	reqBody := req.Body.(map[string]interface{})

	arrMessages := []float64{}
	for k := range messages {
		arrMessages = append(arrMessages, k)
	}

	reqBody["messages"] = arrMessages
	reqBody["type"] = "read_ok"
	reply(req, reqBody)

	return nil
}

func onTopology(req Request) error {
	reqBody := req.Body.(map[string]interface{})
	topology := reqBody["topology"].(map[string]interface{})
	neighbors = convertArrInterfaceToArrString(topology[nodeId].([]interface{}))

	logSafe(fmt.Sprintf("Neighbors: %v", neighbors))

	delete(reqBody, "topology")
	reqBody["type"] = "topology_ok"
	reply(req, reqBody)

	return nil
}

func onInit(req Request) error {
	reqBody := req.Body.(map[string]interface{})
	nodeId = reqBody["node_id"].(string)
	nodeIds = convertArrInterfaceToArrString(reqBody["node_ids"].([]interface{}))

	reqBody["type"] = "init_ok"
	reply(req, reqBody)

	return nil
}

func reply(req Request, reqBody map[string]interface{}) {
	body := req.Body.(map[string]interface{})

	reqBody["in_reply_to"] = body["msg_id"]

	sendSafe(req.Src, reqBody)
}

func sendSafe(dest string, body interface{}) {
	res := Response{
		Src:  nodeId,
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

func convertArrInterfaceToArrString(arr []interface{}) []string {
	newArr := make([]string, len(arr))
	for i, v := range arr {
		newArr[i] = v.(string)
	}

	return newArr
}

func rpc(dest string, reqBody map[string]interface{}, callbackHandler func(req Request) error) {
	callbackLock.Lock()
	defer callbackLock.Unlock()
	nextMsgId++
	msgId := nextMsgId
	callbacks[msgId] = callbackHandler
	reqBody["msg_id"] = msgId
	sendSafe(dest, reqBody)
}

func removeElement(arr []string, value string) []string {
	for i := 0; i < len(arr); i++ {
		if arr[i] == value {
			if i == len(arr)-1 {
				return arr[:len(arr)-1]
			}
			return append(arr[:i], arr[i+1:]...)
		}
	}
	return arr
}
