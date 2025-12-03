package main

import (
	//"errors"
	"fmt"
	"time"

	//"log"
	"net"
	"net/rpc"
	"os"
	"raft/internal/comun/check"
	"raft/internal/comun/rpctimeout"
	"raft/internal/raft"
	"strconv"
	"strings"
	//"time"
)

func main() {
	rawID := os.Args[1] // Ej: "raft-0"
	idStr := strings.TrimPrefix(rawID, "raft-")
	me, err := strconv.Atoi(idStr)
	check.CheckError(err, "Main: fallo convirtiendo ID del nodo")
	fmt.Printf("Replica %s iniciada como nodo %d\n", rawID, me)
	var nodos []rpctimeout.HostPort
	// Resto de argumento son los end points como strings
	// De todas la replicas-> pasarlos a HostPort
	for _, endPoint := range os.Args[2:] {
		nodos = append(nodos, rpctimeout.HostPort(endPoint))
	}

	// Parte Servidor
	nr := raft.NuevoNodo(nodos, me, make(chan raft.AplicaOperacion, 1000))
	rpc.Register(nr)

	// fmt.Println("Métodos RPC registrados:")
	// t := reflect.TypeOf(nr)
	// for i := 0; i < t.NumMethod(); i++ {
	// 	fmt.Println("-", t.Method(i).Name)
	// }

	fmt.Println("Replica escucha en :", me, " de ", os.Args[2:])

	puerto := ":29280"
	var l net.Listener
	for {
		fmt.Println("Intentando abrir puerto", puerto)
		var err error
		l, err = net.Listen("tcp", puerto)
		if err == nil {
			fmt.Println("✔ Puerto disponible → servidor escuchando en", puerto)
			break
		}

		fmt.Println("❌ Fallo en listen:", err)
		time.Sleep(2 * time.Second)
	}

	rpc.Accept(l)
}
