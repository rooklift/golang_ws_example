package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

type Message struct {
	Type		string				`json:"type"`
	Content		string				`json:"content"`
}

type Connection struct {
	Conn		*websocket.Conn
	Id			int
	InChan		chan *Message		// Chan on which incoming messages are placed.
	OutChan		chan *Message		// Chan on which outgoing messages are placed.
}

type ConnIdGenerator struct {
	val			int
}

func (self *ConnIdGenerator) Next() int {
	self.val += 1
	return self.val
}

var upgrader = websocket.Upgrader{CheckOrigin: check_origin}
var conn_id_generator = ConnIdGenerator{}

var new_conn_chan = make(chan *Connection, 64)
var dead_conn_chan = make(chan *Connection, 64)

func check_origin(r *http.Request) bool {				// FIXME
	return true
}

func hub() {

	var connections []*Connection

	for {

		select {

		case new_conn := <- new_conn_chan:

			connections = append(connections, new_conn)
			new_conn.OutChan <- &Message{Type: "debug", Content: fmt.Sprintf("Hello client %d", new_conn.Id)}

		case dead_conn := <- dead_conn_chan:

			// Note we likely receive multiple notifications of this...

			for i := len(connections) - 1; i >= 0; i-- {
				if connections[i] == dead_conn {
					connections = append(connections[:i], connections[i + 1:]...)
				}
			}

		default:
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func connection_io(w http.ResponseWriter, r *http.Request) {

	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Printf("%v\n", err)
		return
	}

	conn_info := Connection{
		Conn:		c,
		Id:			conn_id_generator.Next(),
		InChan:		make(chan *Message, 64),		// Chan on which incoming messages are placed.
		OutChan:	make(chan *Message, 64),		// Chan on which outgoing messages are placed.
	}
	defer conn_info.Conn.Close()

	go read_loop(&conn_info)
	new_conn_chan <- &conn_info

	MainLoop:
	for {
		select {

		// Messages from client...
		// We could alternatively handle these in hub()

		case msg := <- conn_info.InChan:
			fmt.Printf("%s: %s\n", msg.Type, msg.Content);

		// Messages to client...

		case msg := <- conn_info.OutChan:

			b, err := json.Marshal(msg)
			if err != nil {
				fmt.Printf("%v\n", err)
				break						// Breaks the case only.
			}

			err = c.WriteMessage(websocket.TextMessage, b)
			if err != nil {
				fmt.Printf("%v\n", err)
				break MainLoop				// Presumably, client disconnected.
			}

		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	dead_conn_chan <- &conn_info
}

func read_loop(conn_info *Connection) {

	for {

		_, b, err := conn_info.Conn.ReadMessage()

		if err != nil {						// Presumably, client disconnected.
			dead_conn_chan <- conn_info
			return
		}

		msg := new(Message)
		err = json.Unmarshal(b, msg)

		if err != nil {
			fmt.Printf("%v\n", err)
		} else {
			conn_info.InChan <- msg
		}
	}
}

func main() {

	go hub()

	http.HandleFunc("/", connection_io)
	http.ListenAndServe("127.0.0.1:8080", nil)
}