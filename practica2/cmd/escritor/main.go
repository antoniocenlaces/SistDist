package main

import (
	"fmt"
	"log"
	"os"
	"practica2/cmd/fileManager"
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

	if len(args) != 5 {
		log.Println("usage: main.go peers me endpointsfile path")
		os.Exit(1)
	}

	peersFile := args[1]
	me, err := strconv.Atoi(args[2])
	checkError(err)
	endpointsFile := args[3]
	path := args[4]
	fileManager := fileManager.New(me, endpointsFile, path, peersFile, false)

	fileManager.ServerOn()

}
