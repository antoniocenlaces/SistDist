package main

import (
	"fmt"
	"log"
	"net/rpc"
	"os"
	"raft/internal/comun/check"
	"raft/internal/comun/rpctimeout"
	"raft/internal/raft"
	"strconv"
	"time"
)

func main() {

	if len(os.Args) < 3 {
		fmt.Println("Uso: cliente #cliente <endpoint1> <endpoint2> ...")
		return
	}

	me, err := strconv.Atoi(os.Args[1])
	check.CheckError(err, "Main, mal numero entero de indice de cliente:")

	var nodos []rpctimeout.HostPort
	for _, e := range os.Args[2:] {
		nodos = append(nodos, rpctimeout.HostPort(e))
	}
	log.SetFlags(log.Lshortfile | log.Lmicroseconds)
	log.Printf("[CLIENTE %d] Iniciado con endpoints: %v", me, nodos)
	// Activa timers en todos los nodos
	// for i, node := range nodos{

	// }
	// ===============================
	// DESCUBRIR LÍDER
	// ===============================

	lider := descubrirLider(nodos)
	log.Printf("[CLIENTE %d] Líder inicial detectado: %d", me, lider)

	// ===============================
	// ENVIAR OPERACIONES AL LÍDER
	// ===============================
	contador := 0
	for {
		op := raft.TipoOperacion{
			Operacion: "escribir",
			Clave:     fmt.Sprintf("C%d:k%d", me, contador),
			Valor:     fmt.Sprintf("v%d", contador),
		}

		log.Printf("[CLIENTE %d] Enviando operación %+v al líder %d", me, op, lider)

		res, nuevoLider := enviarOperacion(nodos, lider, op)

		// Si el cliente no ha podido contactar o el nodo dice "no soy líder"
		if nuevoLider != -1 {
			log.Printf("[CLIENTE %d] ❗ Nuevo líder detectado: %d (antes era %d)", me, nuevoLider, lider)
			lider = nuevoLider
			continue
		}

		if res == nil {
			log.Printf("[CLIENTE %d] ❗ Líder desconocido: redescubriendo...", me)
			lider = descubrirLider(nodos)
			continue
		}

		log.Printf("[CLIENTE %d] ✔ Respuesta del líder %d: %+v", me, lider, res)

		contador++
		time.Sleep(1 * time.Second)
	}
}

// ==========================================
// DESCUBRIR LÍDER CICLANDO POR LOS NODOS
// ==========================================
func descubrirLider(nodos []rpctimeout.HostPort) int {
	for {
		for i, n := range nodos {
			client, err := rpc.Dial("tcp", string(n))
			if err != nil {
				log.Printf("[descubrirLider] Nodo %d no responde (%v)", i, err)
				continue
			}

			var est raft.EstadoRemoto
			err = client.Call("NodoRaft.ObtenerEstadoNodo", raft.Vacio{}, &est)
			client.Close()

			if err != nil {
				log.Printf("[descubrirLider] Error RPC con nodo %d: %v", i, err)
				continue
			}

			if est.EsLider {
				log.Printf("[descubrirLider] ✔ Líder encontrado: %d", est.IdNodo)
				return est.IdNodo
			}
		}

		log.Printf("[descubrirLider] ❗ No se encontró líder, reintentando...")
		time.Sleep(500 * time.Millisecond)
	}
}

// ==========================================
// ENVÍA UNA OPERACIÓN AL LÍDER
// ==========================================
func enviarOperacion(nodos []rpctimeout.HostPort, lider int, op raft.TipoOperacion) (*raft.ResultadoRemoto, int) {
	client, err := rpc.Dial("tcp", string(nodos[lider]))
	if err != nil {
		log.Printf("[enviarOperacion] ❌ Fallo al conectar con nodo %d (%v)", lider, err)
		return nil, -1
	}
	defer client.Close()

	var res raft.ResultadoRemoto
	err = client.Call("NodoRaft.SometerOperacionRaft", op, &res)
	if err != nil {
		log.Printf("[enviarOperacion] ❌ Error RPC al enviar a nodo %d: %v", lider, err)
		return nil, -1
	}

	if !res.EsLider {
		log.Printf("[enviarOperacion] ⚠ Nodo %d dice que NO es líder. El líder real es: %d",
			lider, res.IdLider)
		if res.IdLider >= 0 {
			return nil, res.IdLider
		}
		return nil, -1
	}

	return &res, -1
}
