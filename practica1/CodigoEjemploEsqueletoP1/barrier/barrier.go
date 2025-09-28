package main

import (
	"bufio"
	"errors"
	"net"
	"os"
	"practica1/logger"
	"strconv"
	"sync"
	"time"
)

func readEndpoints(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var endpoints []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			endpoints = append(endpoints, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return endpoints, nil
}

func handleConnection(conn net.Conn, barrierChan chan<- bool,
	received *map[string]bool, mu *sync.Mutex, n int) {
	defer conn.Close()
	buf := make([]byte, 1024)
	_, err := conn.Read(buf)
	if err != nil {
		logger.Log.Error("Error reading from connection:", err)
		return
	}
	msg := string(buf)
	logger.Log.Info("He leido")
	mu.Lock()
	(*received)[msg] = true
	logger.Log.Info("Received ", len(*received), " elements")
	if len(*received) == n-1 {
		barrierChan <- true
	}
	mu.Unlock()
}

// Get enpoints (IP adresse:port for each distributed process)
func getEndpoints() ([]string, int, error) {
	endpointsFile := os.Args[1]
	var endpoints []string // Por qué esta declaración ?
	var err error
	lineNumber, err := strconv.Atoi(os.Args[2])
	if err != nil || lineNumber < 1 {
		logger.Log.Error("Invalid line number")
	} else if endpoints, err = readEndpoints(endpointsFile); err != nil {
		logger.Log.Error("Error reading endpoints:", err)
	} else if lineNumber > len(endpoints) {
		logger.Log.Error("Line number ", lineNumber, "out of range\n")
		err = errors.New("Line number out of range")
	}
	return endpoints, lineNumber, err
}

func acceptAndHandleConnections(listener net.Listener, quitChannel chan bool,
	barrierChan chan bool, receivedMap *map[string]bool, mu *sync.Mutex, nEndpoints int) {

	for {
		select {
		case <-quitChannel:
			logger.Log.Info("Stopping the listener...")
			return
		default:
			conn, err := listener.Accept()
			if err != nil {
				logger.Log.Error("Error accepting connection:", err)
				continue
			}
			go handleConnection(conn, barrierChan, receivedMap, mu, nEndpoints)
		}
	}
}

func notifyOtherDistributedProcesses(endPoints []string, lineNumber int, barrierChan chan bool) {
	sendMap := make(map[int]bool)
	for i, ep := range endPoints {
		if i+1 != lineNumber {
			go func(ep string, sendMap *map[int]bool, barrierChan chan bool, n int) {
				for {
					conn, err := net.Dial("tcp", ep)
					if err != nil {
						logger.Log.Error("Error connecting to", ep, ":", err)
						time.Sleep(1 * time.Second)
						continue
					}
					_, err = conn.Write([]byte(strconv.Itoa(lineNumber)))
					if err != nil {
						logger.Log.Error("Error sending message:", err)
						conn.Close()
						continue
					}
					(*sendMap)[i] = true
					logger.Log.Info("Sent msg to ", ep, " sents ", len(*sendMap))
					if n-1 == len(*sendMap) {
						barrierChan <- true
					}

					conn.Close()
					break
				}
			}(ep, &sendMap, barrierChan, len(endPoints))
		}

	}
}

func main() {

	var listener net.Listener //Socket TCP/IP para
	if len(os.Args) != 3 {    //Tiene que haber 3 argumentos
		logger.Log.Warning("Usage: go run main.go <endpoints_file> <line_number>")
	} else if endPoints, lineNumber, err := getEndpoints(); err == nil {
		// Get the endpoint for current process
		localEndpoint := endPoints[lineNumber-1]
		if listener, err = net.Listen("tcp", localEndpoint); err != nil {
			logger.Log.Error("Error creating listener:", err)
		} else {
			logger.Log.Info("Listening on", localEndpoint)
			// Barrier synchronization
			var mu sync.Mutex
			quitChannel := make(chan bool)
			receivedMap := make(map[string]bool)
			barrierChan := make(chan bool)

			go acceptAndHandleConnections(listener, quitChannel, barrierChan, &receivedMap, &mu, len(endPoints))

			notifyOtherDistributedProcesses(endPoints, lineNumber, barrierChan)

			logger.Log.Info("Waiting for all the processes to reach the barrier")
			<-barrierChan
			<-barrierChan
			listener.Close()
			quitChannel <- true

			logger.Log.Info("Exiting from program...")
		}
	}
}
