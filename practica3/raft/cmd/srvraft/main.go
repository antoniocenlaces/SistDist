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
)

func main() {
	// obtener entero de indice de este nodo
	me, err := strconv.Atoi(os.Args[1])
	check.CheckError(err, "Main, mal numero entero de indice de nodo:")

	var nodos []rpctimeout.HostPort
	// Resto de argumento son los end points como strings
	// De todas la replicas-> pasarlos a HostPort
	for _, endPoint := range os.Args[2:] {
		nodos = append(nodos, rpctimeout.HostPort(endPoint))
	}

	// Parte Servidor
	// switch me {
	// case 0:
	// 	time.Sleep(200 * time.Millisecond)
	// case 1:
	// 	time.Sleep(100 * time.Millisecond)
	// }
	// nr := raft.NuevoNodo(nodos, me, make(chan raft.AplicaOperacion, 1000))
	// rpc := rpc.NewServer()
	canalAplicar := make(chan raft.AplicaOperacion, 1000)
	// --- Monitorear operaciones aplicadas ---
	go func() {
		for ap := range canalAplicar {
			fmt.Printf("[Nodo %d] Aplicada operación: %+v\n", me, ap)
		}
	}()
	nr := raft.NuevoNodo(nodos, me, canalAplicar)
	rpc.Register(nr)

	fmt.Println("Replica escucha en :", me, " de ", os.Args[2:])

	l, err := net.Listen("tcp", os.Args[2:][me])
	check.CheckError(err, "Main listen error:")

	go rpc.Accept(l)

	// Espera más de 2.5 s para garantizar que hay líder
	time.Sleep(5 * time.Second)

	// Cliente de prueba se usa el nodo 0
	if me == 0 {
		nr.Logger.Println("Nodo 0 intenta descubrir el líder actual...")

		var liderIdx = -1
		for _, peer := range nodos {
			client, err := rpc.Dial("tcp", string(peer))
			if err != nil {
				nr.Logger.Printf("No se pudo conectar con %s\n", peer)
				continue
			}
			defer client.Close()

			var reply raft.EstadoRemoto
			err = client.Call("NodoRaft.ObtenerEstadoNodo", raft.Vacio{}, &reply)
			if err == nil && reply.EsLider {
				liderIdx = reply.IdNodo
				nr.Logger.Printf("Líder actual detectado: nodo %d\n", liderIdx)
				break
			}
		}

		if liderIdx == -1 {
			nr.Logger.Println("No se encontró líder, abortando prueba.")
			return
		}

		// Enviar operación de prueba al líder
		client, err := rpc.Dial("tcp", string(nodos[liderIdx]))
		check.CheckError(err, "Dial al líder falló:")

		op := raft.TipoOperacion{
			Operacion: "escribir",
			Clave:     "x",
			Valor:     fmt.Sprintf("valor%d", time.Now().Unix()%1000),
		}

		var res raft.ResultadoRemoto
		err = client.Call("NodoRaft.SometerOperacionRaft", op, &res)
		check.CheckError(err, "Error en llamada SometerOperacionRaft:")

		nr.Logger.Printf("Respuesta de líder %d: índice=%d mandato=%d EsLider=%v IdLider=%d Valor='%s'\n",
			liderIdx, res.IndiceRegistro, res.Mandato, res.EsLider, res.IdLider, res.ValorADevolver)
	}

	select {} // no termina

}
