package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

var (
	nodeId    string
	nodeIds   []string
	nextMsgId int
	handler   map[string]func(req Request) error
	messages  map[float64]bool
	neighbors []string
	logLock   sync.Mutex
	sendLock  sync.Mutex
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

		// check if handler exists
		msgType := reqBody["type"].(string)
		if _, ok := handler[msgType]; !ok {
			logSafe(fmt.Sprintf("No handler for %v", req))
			continue
		}

		// execute handler
		if err := handler[msgType](req); err != nil {
			logSafe(fmt.Sprintf("Error executing handler for %v: %s", req, err))
			continue
		}
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

	if _, ok := messages[message]; !ok {
		messages[message] = true

		for _, neighbor := range neighbors {
			cloneBody := make(map[string]interface{})
			for k, v := range reqBody {
				cloneBody[k] = v
			}
			cloneBody["type"] = "broadcast"
			cloneBody["message"] = message
			delete(cloneBody, "msg_id")
			sendSafe(neighbor, cloneBody)
		}
	}

	if _, ok := reqBody["msg_id"]; ok {
		delete(reqBody, "message")
		reqBody["type"] = "broadcast_ok"
		reply(req, reqBody)
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
