package main

import (
	"fmt"
	"net"
	"os"
	"time"
)

func printEsto(can, sal chan int) {
	time.Sleep(100 * time.Millisecond)
	for v := range can {
		fmt.Println("Recibido: ", v)
		if v == 3 {
			// close(can)
			sal <- 0
		}
	}
	// sal <- 0
}

func tratarConexion(conn net.Conn) {
	buf := make([]byte, 1024)
	reqLen, err := conn.Read(buf)
	if err != nil {
		fmt.Println("Error en Lectura:", err.Error())
		os.Exit(1)
	}
	fmt.Println("Enviado por cliente y leido por servidor: ", string(buf[:reqLen]))
	conn.Write([]byte("Message recevied"))
}

func main() {
	c := make(chan int)
	quit := make(chan int)
	go printEsto(c, quit)
	for i, num := range [6]int{9, 8, 7, 6, 5, 4} {
		fmt.Printf("Valor de i: %d, Valor de num: %d\n", i, num)
		// time.Sleep(50 * time.Millisecond)
		// Este select no se bloquea en c <- num si al otro lado no se está recibiendo
		select {
		case c <- num:
		case <-quit:
			fmt.Println("Saliendo al recibir por quit")
			return
		default:
			fmt.Println("Esperando recibir por c")
			time.Sleep(50 * time.Millisecond)
		}
	}
	c <- 3
	<-quit
	close(quit)

	fmt.Println("Nuevo bucle con canal bloqueante")
	c = make(chan int)
	quit = make(chan int)
	go printEsto(c, quit)
	for i, num := range [8]int{9, 8, 7, 6, 5, 4, 3, 2} {
		fmt.Printf("Valor de i: %d, Valor de num: %d\n", i, num)
		// time.Sleep(50 * time.Millisecond)
		// Este select no se bloquea en c <- num si al otro lado no se está recibiendo
		select {
		case c <- num:
		case <-quit:
			fmt.Println("Saliendo al recibir por quit")
			goto label
		}
	}
label:
	close(quit)
	var s []int
	s = append(s, 1, 2, 3)
	s = append(s, 7, 9, 11)
	fmt.Println(s)
	sp := s[2:4]
	fmt.Println(sp)

	// Servidor que escucha para recibir mensaje en loopback y lo devuelve
	l, err := net.Listen("tcp", "127.0.0.1:9999")
	if err != nil {
		fmt.Println("Error listening:", err.Error())
		os.Exit(1)
	}
	defer l.Close()
	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error en Accept:", err.Error())
			os.Exit(1)
		}
		go tratarConexion(conn)
	}
}
