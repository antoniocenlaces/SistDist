package main

import (
	"fmt"
	"log"
	"os"
	"practica2/ra"
	"strconv"
)

func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		os.Exit(1)
	}
}

func writeFile() {

}

func main() {
	args := os.Args

	if len(args) != 3 {
		log.Println("usage: main.go peers me path")
		os.Exit(1)
	}

	peersFile := args[1]
	me, err := strconv.Atoi(args[2])
	checkError(err)
	path := args[3]

	distributedMutex := ra.New(me, peersFile, false)
	for {
		distributedMutex.PreProtocol()

		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s", err.Error())
		} else {
			log.Println("File content: ", data)
		}
		distributedMutex.PostProtocol()
	}

}
