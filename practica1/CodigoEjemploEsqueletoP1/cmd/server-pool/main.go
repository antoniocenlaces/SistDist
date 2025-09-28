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
	"encoding/gob"
	"log"
	"net"
	"os"
	"practica1/com"
	"strconv"
)

// PRE: verdad = !foundDivisor
// POST: IsPrime devuelve verdad si n es primo y falso en caso contrario
func isPrime(n int) (foundDivisor bool) {
	foundDivisor = false
	for i := 2; (i < n) && !foundDivisor; i++ {
		foundDivisor = (n%i == 0)
	}
	return !foundDivisor
}

// PRE: interval.A < interval.B
// POST: FindPrimes devuelve todos los números primos comprendidos en el
//
//	intervalo [interval.A, interval.B]
func findPrimes(interval com.TPInterval) (primes []int) {
	for i := interval.Min; i <= interval.Max; i++ {
		if isPrime(i) {
			primes = append(primes, i)
		}
	}
	return primes
}

func processRequest(requestChan chan com.Request, replyChan chan com.Reply, quit chan bool) {
	for {
		select {
		case request := <-requestChan:
			primes := findPrimes(request.Interval)
			reply := com.Reply{Id: request.Id, Primes: primes}
			replyChan <- reply
		case <-quit:
			return
		}
	}
}
func handleConnection(conn net.Conn, requestConn *map[int]net.Conn,
	readChan chan com.Request, quit chan bool, listner *net.Listener) {

	log.Println("New connection from ", conn.RemoteAddr())
	var request com.Request
	decoder := gob.NewDecoder(conn)
	err := decoder.Decode(&request)
	com.CheckError(err)
	if request.Id == -1 {
		close(quit)
		(*listner).Close()
	} else {
		(*requestConn)[request.Id] = conn
		readChan <- request
	}

}
func replyHandle(requestConn *map[int]net.Conn, replyChan chan com.Reply, quit chan bool) {

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

func main() {
	args := os.Args
	if len(args) != 3 {
		log.Println("Error: endpoint missing: go run server.go ip:port nPool")
		os.Exit(1)
	}
	endpoint := args[1]
	listener, err := net.Listen("tcp", endpoint)

	com.CheckError(err)
	defer listener.Close()
	log.SetFlags(log.Lshortfile | log.Lmicroseconds)

	log.Println("***** Listening for new connection in endpoint ", endpoint)
	readChan := make(chan com.Request)
	replychan := make(chan com.Reply)
	replyMap := make(map[int]net.Conn)
	quitChan := make(chan bool)
	n, err := strconv.Atoi(args[2])
	com.CheckError(err)
	for i := 0; i < n; i++ {
		go processRequest(readChan, replychan, quitChan)
	}
	go replyHandle(&replyMap, replychan, quitChan)
	for {
		conn, err := listener.Accept()
		select {
		case <-quitChan:
			log.Println("Servidor Detenido")
			return
		default:
			com.CheckError(err)
			go handleConnection(conn, &replyMap, readChan, quitChan, &listener)
		}

	}

}
