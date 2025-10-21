package main

import (
	"bufio"
	"fmt"
	"io"
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
	fm := fileManager.New(me, endpointsFile, path, peersFile, false)
	reader := bufio.NewReader(os.Stdin)
	go fm.ServerOn()
	for {
		fmt.Print("Introduce lo que quieras escribir: ")
		input, _ := reader.ReadString('\n')
		if input != "0" {
			err = fm.CallWrite(me, input, 0, io.SeekEnd)
			if err == nil {
				log.Println("Soy ", me, " he escrito correctamente")
			} else {
				log.Println("Error en ", me, " escribiendo en fichero")
			}
		} else {
			fm.Close()
			break
		}
	}

}
