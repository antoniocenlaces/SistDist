// Lanza cuatro nodos y sus endpoints de RPC en cuatro IP's diferentes
package main

import (
	"fmt"
	"log"
	"os"
	fileManagerServer "practica2/cmd/fileManager/server"
	"strconv"
)

func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		os.Exit(1)
	}
}

func createFile(path, content string) {
	os.WriteFile(path, []byte(content), 0644)
}

func createConfigFile(path string, lines []string) {
	f, _ := os.Create(path)
	for _, line := range lines {
		f.WriteString(line + "\n")
	}
	f.Close()
}

func main() {
	args := os.Args
	log.SetFlags(log.Lshortfile | log.Lmicroseconds)

	if len(args) != 3 {
		log.Println("usage: main.go myNodeNumber reader")
		os.Exit(1)
	}
	me, err := strconv.Atoi(args[1])
	checkError(err)
	reader, err := strconv.Atoi(args[2])
	checkError(err)
	if reader < 0 || reader > 1 {
		log.Println("reader=0 => is a writer; reader=1 => is a reader")
		os.Exit(1)
	}
	// 						reader				writer					reader				writer
	endpoints := []string{"127.0.0.1:29280", "127.0.0.1:29281", "127.0.0.1:29282", "127.0.0.1:29283"}
	peers := []string{"127.0.0.1:29284", "127.0.0.1:29285", "127.0.0.1:29286", "127.0.0.1:29287"}

	endpointsFile := "test_endpoints.txt"
	peersFile := "test_peers.txt"
	testFile1 := "test_data_node" + strconv.Itoa(me) + ".txt"

	createConfigFile(endpointsFile, endpoints)
	createConfigFile(peersFile, peers)
	createFile(testFile1, "")

	// Inicializar dos nodo
	fs1 := fileManagerServer.New(me, endpointsFile, testFile1, peersFile, (reader != 0)) // lector

	fs1.Listen()

	defer fs1.Close()

}
