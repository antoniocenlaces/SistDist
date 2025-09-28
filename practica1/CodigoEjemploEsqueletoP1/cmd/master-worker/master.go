/*
* AUTOR: Rafael Tolosana Calasanz y Unai Arronategui
* ASIGNATURA: 30221/39521 - Sistemas Distribuidos
*	       Escuela de Ingeniería y Arquitectura - Universidad de Zaragoza
* FECHA: septiembre de 2022
* FICHERO: server-draft.go
* DESCRIPCIÓN: contiene la funcionalidad esencial para realizar los servidores
*				correspondientes a la práctica 1
 */
package main

import (
	"bufio"
	"encoding/gob"
	"log"
	"net"
	"os"
	"practica1/com"
	"sync"
)

var mtxNGOS sync.Mutex
var nGOs int = 0
var finSync chan bool = make(chan bool)

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

// Get enpoints (IP adresse:port for each distributed process)
func getEndpoints() ([]string, error) {
	endpointsFile := os.Args[2]
	var endpoints []string // Por qué esta declaración ?
	var err error

	if endpoints, err = readEndpoints(endpointsFile); err != nil {
		log.Println("Error reading endpoints:", err)
	}
	return endpoints, err
}

func processRequest(endpoint string, requestChan chan com.Request, replyChan chan com.Reply, quit chan bool) {
	incrementarGos()
	defer decrementarGos()
	for {
		select {
		case request := <-requestChan:
			var reply com.Reply
			conn, err := net.Dial("tcp", endpoint)
			com.CheckError(err)
			encoder := gob.NewEncoder(conn)
			err = encoder.Encode(request)
			com.CheckError(err)
			log.Println("Request enviado al worker ", conn.RemoteAddr())

			decoder := gob.NewDecoder(conn)
			err = decoder.Decode(&reply)
			com.CheckError(err)
			log.Println("Reply recivida del worker ", conn.RemoteAddr())
			replyChan <- reply
		case <-quit:
			fin := com.Request{Id: -1, Interval: com.TPInterval{Min: 0, Max: 0}}
			conn, err := net.Dial("tcp", endpoint)
			com.CheckError(err)
			encoder := gob.NewEncoder(conn)
			err = encoder.Encode(fin)
			com.CheckError(err)
			log.Println("Request de fin enviado al worker ", conn.RemoteAddr())
			return
		}
	}
}
func handleConnection(conn net.Conn, requestConn *map[int]net.Conn,
	readChan chan com.Request, quit chan bool, listner *net.Listener, mu *sync.Mutex) {
	incrementarGos()
	defer decrementarGos()

	log.Println("New connection from ", conn.RemoteAddr())
	var request com.Request
	decoder := gob.NewDecoder(conn)
	err := decoder.Decode(&request)
	com.CheckError(err)
	if request.Id == -1 {
		log.Println("Request de fin de programa")
		close(quit)
		(*listner).Close()
	} else {
		mu.Lock()
		(*requestConn)[request.Id] = conn
		mu.Unlock()
		readChan <- request
	}

}
func replyHandle(requestConn *map[int]net.Conn, replyChan chan com.Reply, quit chan bool) {
	incrementarGos()
	defer decrementarGos()
	for {
		select {
		case reply := <-replyChan:
			if conn, ok := (*requestConn)[reply.Id]; ok {
				encoder := gob.NewEncoder(conn)
				encoder.Encode(&reply)
				delete(*requestConn, reply.Id)
				conn.Close()
				log.Println("Reply to", conn.RemoteAddr())
			}
		case <-quit:
			return
		}
	}
}

func incrementarGos() {
	defer mtxNGOS.Unlock()

	mtxNGOS.Lock()
	nGOs++

}
func decrementarGos() {
	defer mtxNGOS.Unlock()

	mtxNGOS.Lock()
	nGOs--
	if nGOs == 0 {
		finSync <- true
	}
}

func main() {
	args := os.Args

	if len(args) != 3 {
		log.Println("Error: endpoint missing: go run server.go ip:port endpoint-file")
		os.Exit(1)
	}
	endPoints, err := getEndpoints()
	com.CheckError(err)
	endpoint := args[1]
	listener, err := net.Listen("tcp", endpoint)
	com.CheckError(err)
	defer listener.Close()
	log.SetFlags(log.Lshortfile | log.Lmicroseconds)

	log.Println("***** Listening for new connection in endpoint ", endpoint)
	var mu sync.Mutex
	readChan := make(chan com.Request)
	replychan := make(chan com.Reply)
	replyMap := make(map[int]net.Conn)
	quitChan := make(chan bool)

	for i := range endPoints {
		log.Println("Gorutine para ", endPoints[i])
		go processRequest(endPoints[i], readChan, replychan, quitChan)
	}
	go replyHandle(&replyMap, replychan, quitChan)
	for {
		conn, err := listener.Accept()
		select {
		case <-quitChan:
			log.Println("Deteniendo servidor....")
			<-finSync
			log.Println("Exiting program")
			return
		default:
			com.CheckError(err)
			go handleConnection(conn, &replyMap, readChan, quitChan, &listener, &mu)
		}

	}

}
