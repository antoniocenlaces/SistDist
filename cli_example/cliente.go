package main

import (
	"fmt"
	"net"
	"os"
)

func main() {
	conn, err := net.Dial("tcp", "127.0.0.1:9999")
	if err != nil {
		fmt.Println("Error en estableciendo conexión desde el cliente:", err.Error())
		os.Exit(1)
	}
	defer conn.Close()
	_, err = conn.Write([]byte("Hola!"))
	if err != nil {
		fmt.Println("Error en escritura del cliente:", err.Error())
		os.Exit(1)
	}
	buffer := make([]byte, 1024)
	mLen, err := conn.Read(buffer)
	if err != nil {
		fmt.Println("Error en lectura del cliente:", err.Error())
		os.Exit(1)
	}
	fmt.Println("Recibido por cliente: ", string(buffer[:mLen]))
}
