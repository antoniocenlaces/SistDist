package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	fileManagerClient "practica2/cmd/fileManager/client"
	fileManagerServer "practica2/cmd/fileManager/server"
	"strconv"
)

func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		os.Exit(1)
	}
}

func main() {
	args := os.Args
	log.SetFlags(log.Lshortfile | log.Lmicroseconds)

	if len(args) != 5 {
		log.Println("usage: main.go peers me endpointsfile path")
		os.Exit(1)
	}

	peersFile := args[1]
	me, err := strconv.Atoi(args[2])
	checkError(err)
	endpointsFile := args[3]
	path := args[4]
	fm := fileManagerServer.New(me, endpointsFile, path, peersFile, true)
	reader := bufio.NewReader(os.Stdin)
	go fm.Listen()
	for {
		fmt.Print("Quieres leer [y\\n]: ")
		input, _ := reader.ReadString('\n')
		if input != "n\n" {
			data, err := fileManagerClient.CallRead(fm.LocalAdress(), 0, 0)
			if err == nil {
				log.Println("Leido {\n", data, "\n}")
			} else {
				log.Println("Error en ", me, " leyendo de fichero")
			}
		} else {
			fm.Close()
			break
		}
	}

}
