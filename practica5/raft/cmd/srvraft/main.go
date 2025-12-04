package main

import (
	//"errors"
	"fmt"
	"sync"
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

	go rpc.Accept(l)

	// ---------- Goroutine que hace ping periódicamente a todos los nodos ----------
	go func() {
		pingTimeout := 1 * time.Second         // timeout para cada llamada Ping
		pollInterval := 500 * time.Millisecond // intervalo entre rondas de ping
		globalTimeout := 30 * time.Second      // tiempo máximo esperando a todos ready

		deadline := time.Now().Add(globalTimeout)
		up := make(map[rpctimeout.HostPort]bool)
		var mu sync.Mutex

		var replyAct raft.Vacio

		for {
			// comprobar timeout global
			if time.Now().After(deadline) {
				fmt.Println("Timeout esperando que todos los nodos estén READY; activando timers local igualmente")
				// decisión: activar timers aún si no todos están listos
				nr.ActivarTimers(raft.Vacio{}, &replyAct)
				return
			}

			var wg sync.WaitGroup
			for _, hp := range nodos {
				h := hp
				wg.Add(1)
				go func() {
					defer wg.Done()
					var reply raft.PingReply
					err := h.CallTimeout("NodoRaft.Ping", raft.Vacio{}, &reply, pingTimeout)
					mu.Lock()
					if err == nil && reply.Ready {
						up[h] = true
					} else {
						up[h] = false
					}
					mu.Unlock()
				}()
			}
			wg.Wait()

			// comprobar si todos están up
			allUp := true
			mu.Lock()
			for _, h := range nodos {
				if !up[h] {
					allUp = false
					break
				}
			}
			mu.Unlock()

			if allUp {
				fmt.Println("Todos los nodos responden READY → activando timers")
				nr.ActivarTimers(raft.Vacio{}, &replyAct)
				return
			}

			time.Sleep(pollInterval)
		}
	}()

	// Evitamos que main termine
	select {}
}
