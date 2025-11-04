package extrap3

import (
	"log"
	"sync"
	"time"

	"../practica3/raft/internal/comun/rpctimeout"
)

type TipoOperacion struct {
	Operacion string // La operaciones posibles son "leer" y "escribir"
	Clave     string
	Valor     string // en el caso de la lectura Valor = ""
}

// A medida que el nodo Raft conoce las operaciones de las  entradas de registro
// comprometidas, envía un AplicaOperacion, con cada una de ellas, al canal
// "canalAplicar" (funcion NuevoNodo) de la maquina de estados
type AplicaOperacion struct {
	Indice    int // en la entrada de registro
	Operacion TipoOperacion
}
type NodoRaft struct {
	Mux sync.Mutex // Mutex para proteger acceso a estado compartido

	// Host:Port de todos los nodos (réplicas) Raft, en mismo orden
	Nodos   []rpctimeout.HostPort
	Yo      int         // indice de este nodos en campo array "nodos"
	IdLider int         // índice en Nodos del líder
	Logger  *log.Logger // para facilitar depuración

	//

	// Estado persistente en todos los nodos
	currentTerm int               // último mandato visto por el nodo. 0 al inicializar.
	votedFor    int               // candidato que recibió mi voto en mandato actual
	logEntries  []AplicaOperacion // registro de entradas en máquina estados

	// Estado volatil
	commitIndex int // índice de la mayor entrada en logEntries que ha de ser
	// aplicada. Inicializa a 0.
	lastApplied int // índice de la mayor entrada en logEntries que ha sido
	// aplicada.

	// Solo en líder
	nextIndex map[int]int // para cada servidor, índice de la siguiente
	// entrada a ser enviada a ese servidor (inicializa a último índice
	// de logEntries del líder + 1)
	matchIndex map[int]int // para cada servidor, almacena el índice
	// más alto conocido a ser replicado en ese servidor

	// Estado temporal (no persistente)
	role          string        // "Follower", "Candidate", "Leader"
	votesReceived int           // votos durante elección
	electionReset time.Duration // última vez que recibió mensaje válido
	timer         *time.Timer   // para gestionar timeout de elecciones
	// Canal para aplicar operaciones comprometidas a la máquina de estados
	canalAplicarOperacion chan AplicaOperacion
}

func (nr *NodoRaft) RecibirPeticionVoto(args *ArgsPeticionVoto, reply *RespuestaPeticionVoto) error {
	nr.Mux.Lock()
	defer nr.Mux.Unlock()

	reply.Term = nr.currentTerm
	reply.VoteGranted = false // por defecto, no voto

	// Si el término del candidato es menor, rechazo directamente
	if args.Term < nr.currentTerm {
		nr.Logger.Printf("[Nodo %d] Rechaza voto a %d por término menor (%d < %d)",
			nr.Yo, args.CandidateId, args.Term, nr.currentTerm)
		return nil
	}

	// Si el término del candidato es mayor, actualizo mi término y paso a seguidor
	if args.Term > nr.currentTerm {
		nr.currentTerm = args.Term
		nr.votedFor = -1
		nr.role = "Follower"
	}

	// Compruebo si ya voté en este término
	if nr.votedFor == -1 || nr.votedFor == args.CandidateId {
		// Verifico si el log del candidato está al día (simplificado)
		lastLogIndex := len(nr.logEntries) - 1
		lastLogTerm := 0
		if lastLogIndex >= 0 {
			lastLogTerm = nr.logEntries[lastLogIndex].Term // si AplicaOperacion tiene campo Term
		}

		upToDate := (args.LastLogTerm > lastLogTerm) ||
			(args.LastLogTerm == lastLogTerm && args.LastLogIndex >= lastLogIndex)

		if upToDate {
			reply.VoteGranted = true
			nr.votedFor = args.CandidateId
			nr.Logger.Printf("[Nodo %d] Vota a %d en término %d", nr.Yo, args.CandidateId, args.Term)
		}
	}

	reply.Term = nr.currentTerm
	return nil
}
