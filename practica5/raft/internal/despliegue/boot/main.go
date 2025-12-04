package main

import (
	"log"
	"net/rpc"
	"os/exec"
	"raft/internal/comun/rpctimeout"
	"raft/internal/raft"
	"time"
)

func activarTimers(nodos []rpctimeout.HostPort) {
	for i, n := range nodos {
		client, err := rpc.Dial("tcp", string(n))
		if err != nil {
			log.Fatalf("Error conectando a nodo %d: %v", i, err)
		}
		var r raft.Vacio
		err = client.Call("NodoRaft.ActivarTimers", raft.Vacio{}, &r)
		client.Close()
		log.Printf("Timers activados en nodo %d", i)
	}
}

func main() {
	log.Println("Esperando que Raft esté Ready...")

	cmd := exec.Command("kubectl", "wait",
		"--for=condition=Ready", "pod",
		"-l", "app=raft", "--timeout=180s")

	err := cmd.Run()
	if err != nil {
		log.Fatalf("Los nodos Raft no están listos a tiempo")
	}

	time.Sleep(2 * time.Second)

	nodos := []rpctimeout.HostPort{
		"raft-0.raft-svc:29280",
		"raft-1.raft-svc:29280",
		"raft-2.raft-svc:29280",
	}

	activarTimers(nodos)
	log.Println("Cluster Raft inicializado → Elecciones activadas")
}
