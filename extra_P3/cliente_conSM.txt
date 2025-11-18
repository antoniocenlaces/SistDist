package main

import (
	//"errors"
	"fmt"

	//"log"
	"net"
	"net/rpc"
	"os"
	"raft/internal/comun/check"
	"raft/internal/comun/rpctimeout"
	"raft/internal/raft"
	"strconv"
)

func main() {
	// obtener entero de indice de este nodo
	me, err := strconv.Atoi(os.Args[1])
	check.CheckError(err, "Main, mal numero entero de indice de nodo:")
	// Parte Servidor: el canal donde Raft enviará operaciones comprometidas

	canalAplicar := make(chan raft.AplicaOperacion, 1000)
	// -----------------------------
	//  MÁQUINA DE ESTADOS SIMPLIFICADA
	// -----------------------------
	go func() {
		for op := range canalAplicar {
			fmt.Printf("[Nodo %d] APLICA operación %v en índice %d\n",
				me, op.Operacion, op.Indice)
		}
	}()

	var nodos []rpctimeout.HostPort
	for _, endPoint := range os.Args[2:] {
		nodos = append(nodos, rpctimeout.HostPort(endPoint))
	}

	nr := raft.NuevoNodo(nodos, me, canalAplicar)
	rpc.Register(nr)

	fmt.Println("Nodo escucha en:", nodos[me])

	l, err := net.Listen("tcp", os.Args[2:][me])
	check.CheckError(err, "Main listen error:")

	rpc.Accept(l)
}
