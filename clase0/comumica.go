package main

import (
	"encoding/gob"
	"fmt"
	"net"
)

func send(data interface{}) error {
	var conn net.Conn
	var err error
	var encoder *gob.Encoder

	conn, err = net.Dial("tcp", "127.0.0.1:5000")
	if err != nil {
		panic("Client connection error!")
	}
	encoder = gob.NewEncoder(conn)
	err = encoder.Encode(data)
	conn.Close()
	return err
}

func main() {
	fmt.Println("Hello, vamos a comunicar")

}
