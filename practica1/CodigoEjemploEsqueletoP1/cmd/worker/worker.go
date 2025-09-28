package main

import (
	"encoding/gob"
	"log"
	"net"
	"os"
	"practica1/com"
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

func processRequest(conn net.Conn, quitChan chan bool, listner *net.Listener) {
	var request com.Request
	decoder := gob.NewDecoder(conn)
	err := decoder.Decode(&request)
	com.CheckError(err)
	if request.Id != -1 {
		primes := findPrimes(request.Interval)

		reply := com.Reply{Id: request.Id, Primes: primes}
		encoder := gob.NewEncoder(conn)
		encoder.Encode(&reply)
		log.Println("Reply to", conn.RemoteAddr())

	} else {
		log.Println("Request de fin de programa")
		close(quitChan)
		(*listner).Close()
	}

}

func main() {
	args := os.Args
	if len(args) != 2 {
		log.Println("Error: endpoint missing: go run server.go ip:port")
		os.Exit(1)
	}
	endpoint := args[1]
	listener, err := net.Listen("tcp", endpoint)

	com.CheckError(err)
	defer listener.Close()
	log.SetFlags(log.Lshortfile | log.Lmicroseconds)
	quitChan := make(chan bool)

	log.Println("***** Listening for new connection in endpoint ", endpoint)
	for {
		conn, err := listener.Accept()
		select {
		case <-quitChan:
			log.Println("Closing worker....")
			return
		default:
			com.CheckError(err)
			log.Println("New connection from ", conn.RemoteAddr())
			processRequest(conn, quitChan, &listener)
		}

	}
}
