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

func processRequest(conn net.Conn) bool {
	var request com.Request
	var end bool
	decoder := gob.NewDecoder(conn)
	err := decoder.Decode(&request) // espera recibir una petición de un cliente
	com.CheckError(err)
	if request.Id > 0 { // si es una petción de tranbajo se procesa
		primes := findPrimes(request.Interval) // busca los primos en ese intervalo
		reply := com.Reply{Id: request.Id, Primes: primes}
		end = false
		encoder := gob.NewEncoder(conn)
		encoder.Encode(&reply)
	} else { // de lo contrario se avisa que hemos terminado
		end = true
	}
	return end
}

func main() {
	var end bool
	end = false
	args := os.Args
	if len(args) != 2 {
		log.Println("Error: endpoint missing: go run server.go ip:port")
		os.Exit(1)
	}
	endpoint := args[1]
	listener, err := net.Listen("tcp", endpoint)
	com.CheckError(err)

	log.SetFlags(log.Lshortfile | log.Lmicroseconds)

	log.Println("***** Listening for new connection in endpoint ", endpoint)
	for !end {
		conn, err := listener.Accept()
		com.CheckError(err)
		defer conn.Close()
		end = processRequest(conn)
	}
}
