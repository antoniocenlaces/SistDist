package main

import (
	"bufio"
	"errors"
	"log"
	"net"
	"os"
	"strconv"
	"sync"
	"time"
)

// PRE: true
// POST: devuelve un vector de string con los endpoints del fichero filename y si no hay algun error devuelve error nil
// y si hay algun error devuelve el vector de endpoints vacio y el error correspondiente
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
		log.Println("Error reading from connection:", err)
		return
	}
	msg := string(buf)
	mu.Lock()
	(*received)[msg] = true
	log.Println("Received ", len(*received), " elements")
	if len(*received) == n-1 {
		barrierChan <- true //Desbloqueo la barrera si ya he leido los mensajes del resto
	}
	mu.Unlock()
}

// PRE: true
// POST: devuelve un vector de string con los endpoints y si no hay algun error devuelve error nil
// y si hay algun error devuelve el vector de endpoints vacio y el error correspondiente
func getEndpoints() ([]string, int, error) {
	endpointsFile := os.Args[1]
	var endpoints []string // Por qué esta declaración ?
	var err error
	lineNumber, err := strconv.Atoi(os.Args[2])
	if err != nil || lineNumber < 1 {
		log.Println("Invalid line number")
	} else if endpoints, err = readEndpoints(endpointsFile); err != nil {
		log.Println("Error reading endpoints:", err)
	} else if lineNumber > len(endpoints) {
		log.Println("Line number ", lineNumber, "out of range")
		err = errors.New("Line number out of range")
	}
	return endpoints, lineNumber, err
}

func acceptAndHandleConnections(listener net.Listener, quitChannel chan bool,
	barrierChan chan bool, receivedMap *map[string]bool, mu *sync.Mutex, nEndpoints int) {

	for {
		select {
		case <-quitChannel:
			log.Println("Stopping the listener...")
			return
		default:
			conn, err := listener.Accept()
			if err != nil {
				log.Println("Error accepting connection:", err)
				continue
			}
			go handleConnection(conn, barrierChan, receivedMap, mu, nEndpoints)
		}
	}
}

func notifyOtherDistributedProcesses(endPoints []string, lineNumber int, barrierChan chan bool) {
	sendMap := make(map[int]bool) // Mapa de a cuantos procesos se les ha enviado el mensaje
	for i, ep := range endPoints {
		if i+1 != lineNumber {
			go func(ep string, sendMap *map[int]bool, barrierChan chan bool, n int) {
				for {
					conn, err := net.Dial("tcp", ep)
					if err != nil {
						log.Println("Error connecting to", ep, ":", err)
						time.Sleep(1 * time.Second)
						continue
					}
					_, err = conn.Write([]byte(strconv.Itoa(lineNumber)))
					if err != nil {
						log.Println("Error sending message:", err)
						conn.Close()
						continue
					}
					(*sendMap)[i] = true
					log.Println("Sent msg to ", ep, " sents ", len(*sendMap))
					if n-1 == len(*sendMap) {
						barrierChan <- true //Desbloqueo la barrera si ya he mandado mis mensaje al resto
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
		log.Println("Usage: go run main.go <endpoints_file> <line_number>")
	} else if endPoints, lineNumber, err := getEndpoints(); err == nil {
		// Get the endpoint for current process
		localEndpoint := endPoints[lineNumber-1]
		if listener, err = net.Listen("tcp", localEndpoint); err != nil {
			log.Println("Error creating listener:", err)
		} else {
			log.Println("Listening on", localEndpoint)
			// Barrier synchronization
			var mu sync.Mutex
			quitChannel := make(chan bool)
			receivedMap := make(map[string]bool)
			barrierChan := make(chan bool)

			go acceptAndHandleConnections(listener, quitChannel, barrierChan, &receivedMap, &mu, len(endPoints))

			notifyOtherDistributedProcesses(endPoints, lineNumber, barrierChan)

			log.Println("Waiting for all the processes to reach the barrier")
			<-barrierChan
			<-barrierChan
			listener.Close()
			quitChannel <- true

			log.Println("Exiting from program...")
		}
	}
}
