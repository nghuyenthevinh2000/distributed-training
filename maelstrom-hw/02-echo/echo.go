package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type Echo struct {
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

var node_id string
var next_msg_id int

func main() {
	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		line := scanner.Text()
		echo := Echo{}
		err := json.Unmarshal([]byte(line), &echo)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
			continue
		}

		fmt.Fprintf(os.Stderr, "Received \"%v\\n\"\n", echo)

		body := echo.Body.(map[string]interface{})

		switch body["type"].(string) {
		case "init":
			node_id = body["node_id"].(string)
			fmt.Fprintf(os.Stderr, "Initialized node %s\n", node_id)
			body["type"] = "init_ok"
			reply(echo, body)
		case "echo":
			fmt.Fprintf(os.Stderr, "Echoing %v\n", echo.Body)
			body["type"] = "echo_ok"
			reply(echo, body)
		}
	}
}

func reply(request Echo, body map[string]interface{}) {
	next_msg_id++
	id := next_msg_id

	res := Response{}
	body["in_reply_to"] = body["msg_id"]
	body["msg_id"] = id
	res.Src = node_id
	res.Dest = request.Src
	res.Body = body

	jsonBytes, err := json.Marshal(res)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Fprintln(os.Stdout, string(jsonBytes))
}
