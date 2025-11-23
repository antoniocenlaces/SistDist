package raft

//
// API
// ===
// Este es el API que vuestra implementación debe exportar
//
// nodoRaft = NuevoNodo(...)
//   Crear un nuevo servidor del grupo de elección.
//
// nodoRaft.Para()
//   Solicitar la parado de un servidor
//
// nodo.ObtenerEstado() (yo, mandato, esLider)
//   Solicitar a un nodo de elección por "yo", su mandato en curso,
//   y si piensa que es el msmo el lider
//
// nodoRaft.SometerOperacion(operacion interface()) (indice, mandato, esLider)

// type AplicaOperacion

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"

	"raft/internal/comun/rpctimeout"
)

type state int

const (
	// Constante para fijar valor entero no inicializado
	IntNOINICIALIZADO = -1
	//  false deshabilita por completo los logs de depuracion
	// Aseguraros de poner kEnableDebugLogs a false antes de la entrega
	kEnableDebugLogs = false
	// Poner a true para logear a stdout en lugar de a fichero
	kLogToStdout = false
	// Para llevar los logs de salida
	// kLogOutputDir = "/misc/alumnos/sd/sd2526/a143045/SistDist/practica3/raft/cmd/srvraft/logs_raft/"
	kLogOutputDir = "./logs_raft/"

	// timers / rates (ajustables)
	baseTimer     = 450
	ceilTimer     = 900
	rpcTimeout    = 100
	heartbeatRate = 90
	Deadline      = 2500

	// Operaciones permitidas en esta versión
	OP1 = "escribir"
	OP2 = "leer"

	Follower state = iota
	Candidate
	Leader
)

// TipoOperacion expuesto a cliente
// Solo se permiten valores de Operacion: OP1 = "escribir" / OP2 = "leer"
type TipoOperacion struct {
	Operacion string
	Clave     string
	Valor     string
}

type logEntry struct {
	Term      int
	Operacion TipoOperacion
}

type AplicaOperacion struct {
	Indice    int
	Operacion TipoOperacion
}

type NodoRaft struct {
	Mux sync.Mutex

	Nodos   []rpctimeout.HostPort
	Yo      int
	IdLider int
	Logger  *log.Logger

	// persistente
	currentTerm int
	votedFor    int
	logEntries  []logEntry        // index base 1 (logEntries[0] es hueco)
	logStore    map[string]string // Diccionario clave valor

	// volátil
	commitIndex int
	lastApplied int

	// líder
	nextIndex  map[int]int
	matchIndex map[int]int

	// temporal
	role                     state
	votesReceived            int
	electionReset            time.Time
	timer                    *time.Timer
	rng                      *rand.Rand
	initialElectionDelayUsed bool
	canalAplicarOperacion    chan AplicaOperacion
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (nr *NodoRaft) lastLogIndex() int {
	return len(nr.logEntries) - 1
}
func (nr *NodoRaft) lastLogTerm() int {
	if len(nr.logEntries) <= 1 {
		return 0
	}
	return nr.logEntries[len(nr.logEntries)-1].Term
}
func (nr *NodoRaft) isCandidateUpToDate(cLastIdx, cLastTerm int) bool {
	myIdx := nr.lastLogIndex()
	myTerm := nr.lastLogTerm()
	if cLastTerm != myTerm {
		return cLastTerm > myTerm
	}
	return cLastIdx >= myIdx
}

// resetTimer: detiene timer previo de forma segura y crea uno nuevo
func (nr *NodoRaft) resetTimer() {
	var dur time.Duration
	if !nr.initialElectionDelayUsed {
		nr.initialElectionDelayUsed = true
		dur = time.Duration(1500+nr.rng.Intn(500)) * time.Millisecond
	} else {
		dur = time.Duration(baseTimer+nr.rng.Intn(ceilTimer-baseTimer)) * time.Millisecond
	}

	// Si existía timer anterior, detenerlo y marcar nil.
	if nr.timer != nil {
		if !nr.timer.Stop() {
			// Si Stop() retornó false, el callback puede estar en ejecución o encolado.
			// Esperamos brevemente para reducir posibilidad de ejecución concurrente.
			time.Sleep(1 * time.Millisecond)
		}
		nr.timer = nil
	}

	// Guardar una copia local de nr para que el callback pueda usar métodos que adquirirán mutex
	nr.timer = time.AfterFunc(dur, func() {
		// Llamada fuera de lock: iniciarEleccion gestionará locking internamente.
		nr.iniciarEleccion()
	})
}

func (nr *NodoRaft) runWatchdog() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	deadline := Deadline * time.Millisecond
	lastOk := time.Now()
	for range ticker.C {
		nr.Mux.Lock()
		ok := nr.role == Leader || time.Since(nr.electionReset) < deadline
		nr.Mux.Unlock()
		if ok {
			lastOk = time.Now()
			continue
		}
		if time.Since(lastOk) >= deadline {
			nr.para()
			log.Fatalf("[WATCHDOG] Sin líder tras %v: abortando", deadline)
		}
	}
}

func (nr *NodoRaft) para() {
	go func() { time.Sleep(5 * time.Millisecond); os.Exit(0) }()
}

func NuevoNodo(nodos []rpctimeout.HostPort, yo int, canalAplicarOperacion chan AplicaOperacion) *NodoRaft {
	nr := &NodoRaft{
		Nodos:                    nodos,
		Yo:                       yo,
		IdLider:                  -1,
		currentTerm:              0,
		votedFor:                 IntNOINICIALIZADO,
		logEntries:               make([]logEntry, 1),
		logStore:                 make(map[string]string),
		commitIndex:              0,
		lastApplied:              0,
		nextIndex:                make(map[int]int),
		matchIndex:               make(map[int]int),
		role:                     Follower,
		votesReceived:            0,
		initialElectionDelayUsed: false,
		electionReset:            time.Now(),
		rng:                      rand.New(rand.NewSource(time.Now().UnixNano() + int64(yo)*10007)),
		canalAplicarOperacion:    canalAplicarOperacion,
	}

	// Logger
	if kEnableDebugLogs {
		prefix := fmt.Sprintf("%s_%s", nodos[yo].Host(), nodos[yo].Port())
		if kLogToStdout {
			nr.Logger = log.New(os.Stdout, prefix+" -> ", log.Lmicroseconds|log.Lshortfile)
		} else {
			os.MkdirAll(kLogOutputDir, os.ModePerm)
			f, err := os.OpenFile(fmt.Sprintf("%s/%s.txt", kLogOutputDir, prefix), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
			if err != nil {
				panic(err)
			}
			nr.Logger = log.New(io.MultiWriter(f), prefix+" -> ", log.Lmicroseconds|log.Lshortfile)
		}
	} else {
		nr.Logger = log.New(io.Discard, "", 0)
	}
	nr.Logger.Println("logger initialized")

	// Inicializar nextIndex/matchIndex
	lastIdx := nr.lastLogIndex()
	for i := range nodos {
		nr.nextIndex[i] = lastIdx + 1
		nr.matchIndex[i] = 0
	}
	nr.Logger.Printf("Nodo: %d valores de Term: %d IdLider: %d", yo, nr.currentTerm, nr.IdLider)
	// nr.resetTimer()
	// go nr.runWatchdog()
	return nr
}

// estado remoto que devuelve ObtenerEstadoNodo RPC
type Vacio struct{}

type EstadoParcial struct {
	Mandato int
	EsLider bool
	IdLider int
}
type EstadoRemoto struct {
	IdNodo int
	EstadoParcial
}

func (nr *NodoRaft) ObtenerEstadoNodo(args Vacio, reply *EstadoRemoto) error {
	nr.Mux.Lock()
	reply.IdNodo = nr.Yo
	reply.Mandato = nr.currentTerm
	reply.EsLider = (nr.Yo == nr.IdLider)
	reply.IdLider = nr.IdLider
	nr.Mux.Unlock()
	return nil
}

// EstadoNodo para poder verificar desde los tests la ejecución de operaciones
type EstadoNodo struct {
	Term        int
	Role        state
	CommitIndex int
	LastApplied int
	LogLength   int
}

// método RPC que permite observar las variables internas de un nodo:
func (nr *NodoRaft) ObtenerEstadoParaTest(args Vacio, reply *EstadoNodo) error {
	nr.Mux.Lock()
	defer nr.Mux.Unlock()

	reply.Role = nr.role
	reply.Term = nr.currentTerm
	reply.CommitIndex = nr.commitIndex
	reply.LastApplied = nr.lastApplied
	reply.LogLength = len(nr.logEntries)
	return nil
}

// En caso de OP2 = leer si la clave requerida no existe se devuelve nil
type ResultadoRemoto struct {
	ValorADevolver *string
	IndiceRegistro int
	EstadoParcial
}

// someterOperacion RPC: cliente somete una operación al nodo local
// en el call-back a este método RPC en caso de operación "leer"
// si reply.ValorADevolver es nil es que la clave solicitada no existe
func (nr *NodoRaft) SometerOperacionRaft(operacion TipoOperacion, reply *ResultadoRemoto) error {
	idx, term, esLider, idLider, valor := nr.someterOperacion(operacion)
	reply.IndiceRegistro = idx
	reply.Mandato = term
	reply.EsLider = esLider
	reply.IdLider = idLider
	reply.ValorADevolver = valor
	return nil
}

func (nr *NodoRaft) ParaNodo(args Vacio, reply *Vacio) error {
	defer nr.para()
	return nil
}

// ActivarTimers: RPC que ha de ser llamada en cada nodo para comenzar
// proceso de elecciones
func (nr *NodoRaft) ActivarTimers(args Vacio, reply *Vacio) error {
	nr.Mux.Lock()
	// Si hay un timer de elección en marcha: llamada duplicada a ActivarTimers
	// no se hace nada
	if nr.timer != nil {
		nr.Mux.Unlock()
		return nil
	}
	nr.initialElectionDelayUsed = false
	nr.electionReset = time.Now()
	nr.resetTimer()
	nr.Mux.Unlock()
	go nr.runWatchdog()
	return nil
}

// para OP1 = escribir
// someterOperacion: si soy líder, se añade la entrada localmente y se devuelve índice.
// Replicación posterior la realiza pushLoop/EnviarLatido
// para OP2 = leer se devuelve el valor asociado a esa clave, si existe, de lo contrario
// devuelve nil en último valor devuelto
func (nr *NodoRaft) someterOperacion(operacion TipoOperacion) (int, int, bool, int, *string) {
	nr.Mux.Lock()
	defer nr.Mux.Unlock()

	mandato := nr.currentTerm
	idLider := nr.IdLider
	if nr.role != Leader {
		return -1, mandato, false, idLider, nil
	}

	// Operación OP2 = leer
	if operacion.Operacion == OP2 {
		valor, ok := nr.logStore[operacion.Clave]
		if !ok {
			return -1, mandato, true, nr.Yo, nil // clave no existe
		}
		return -1, mandato, true, nr.Yo, &valor
	}
	// solo operaciones que añaden registros clave-valor van a nr.logEntries
	// OP1 = escritura => añadir a logEntries
	entry := logEntry{Term: nr.currentTerm, Operacion: operacion}
	nr.logEntries = append(nr.logEntries, entry)
	indice := len(nr.logEntries) - 1

	// actualizar propios índices
	nr.nextIndex[nr.Yo] = indice + 1
	nr.matchIndex[nr.Yo] = indice

	// la replicación queda en pushLoop()
	return indice, mandato, true, nr.Yo, nil
}

// === RPCs del protocolo Raft ===

type ArgsPeticionVoto struct {
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
}
type RespuestaPeticionVoto struct {
	Term        int
	VoteGranted bool
}

func (nr *NodoRaft) PedirVoto(peticion *ArgsPeticionVoto,
	reply *RespuestaPeticionVoto) error {
	nr.Mux.Lock()
	defer nr.Mux.Unlock()

	// Valores por defecto
	reply.Term = nr.currentTerm
	reply.VoteGranted = false

	nr.Logger.Printf("PV -> Nodo %d recibe PeticionVoto de %d Term=%d (localTerm=%d, votedFor=%d, lastLogIdx=%d, lastLogTerm=%d)\n",
		nr.Yo, peticion.CandidateId, peticion.Term, nr.currentTerm, nr.votedFor, nr.lastLogIndex(), nr.lastLogTerm())

	// Si el mandato del candidato es menor al mío, emito voto negativo
	if peticion.Term < nr.currentTerm {
		// asegurar reply.Term correcto antes de devolver
		reply.Term = nr.currentTerm
		nr.Logger.Printf("PV X Nodo %d rechaza voto para nodo %d porque mandato menor (%d < %d)\n",
			nr.Yo, peticion.CandidateId, peticion.Term, nr.currentTerm)
		return nil
	}

	// Si el mandato del candidato es más actual que el mío, me actualizo (y "caigo" a follower)
	if peticion.Term > nr.currentTerm {
		nr.Logger.Printf("PV ! Nodo %d actualiza mandato (%d -> %d) por petición de %d\n",
			nr.Yo, nr.currentTerm, peticion.Term, peticion.CandidateId)
		nr.currentTerm = peticion.Term
		nr.votedFor = IntNOINICIALIZADO
		nr.role = Follower
		// asegurar reply.Term refleja el nuevo término
		reply.Term = nr.currentTerm
	}

	// Regla "up-to-date" del log y política de voto
	if !(nr.votedFor == IntNOINICIALIZADO || nr.votedFor == peticion.CandidateId) {
		reply.Term = nr.currentTerm
		nr.Logger.Printf("PV X Nodo %d ya votó por %d en mandato %d, no concede voto a %d\n",
			nr.Yo, nr.votedFor, nr.currentTerm, peticion.CandidateId)
		return nil
	}

	if !nr.isCandidateUpToDate(peticion.LastLogIndex, peticion.LastLogTerm) {
		reply.Term = nr.currentTerm
		nr.Logger.Printf("PV X Nodo %d rechaza voto a %d: candidato no 'up-to-date' (candLastIdx=%d candLastTerm=%d, miLastIdx=%d miLastTerm=%d)\n",
			nr.Yo, peticion.CandidateId, peticion.LastLogIndex, peticion.LastLogTerm, nr.lastLogIndex(), nr.lastLogTerm())
		return nil
	}

	// Conceder voto
	nr.votedFor = peticion.CandidateId
	reply.VoteGranted = true
	reply.Term = nr.currentTerm // mantener consistente
	nr.resetTimer()
	nr.Logger.Printf("PV ✓ Nodo %d Vota por %d en mandato %d\n",
		nr.Yo, nr.votedFor, nr.currentTerm)
	return nil
}

// AppendEntries (heartbeat + replication)
type ArgAppendEntries struct {
	Term         int
	IdLeader     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []logEntry
	LeaderCommit int
}
type Results struct {
	Term    int
	Success bool
}

func (nr *NodoRaft) AppendEntries(args *ArgAppendEntries, results *Results) error {
	// Lock here to update local state
	nr.Mux.Lock()
	defer nr.Mux.Unlock()

	results.Success = false
	results.Term = nr.currentTerm

	// nr.Logger.Printf("AE <- Nodo %d recibe AppendEntries de líder %d en Term=%d (local=%d) | PrevIdx=%d PrevTerm=%d | len(Entries)=%d LeaderCommit=%d (localCommit=%d)",
	// 	nr.Yo, args.IdLeader, args.Term, nr.currentTerm, args.PrevLogIndex, args.PrevLogTerm, len(args.Entries), args.LeaderCommit, nr.commitIndex)

	// If leader term < currentTerm -> reject
	if args.Term < nr.currentTerm {
		return nil
	}

	// If leader term > currentTerm -> update term and become follower
	if args.Term > nr.currentTerm {
		// nr.Logger.Printf("AE ! Nodo %d actualiza mandato (%d -> %d) y reconoce a %d como nuevo líder",
		// nr.Yo, nr.currentTerm, args.Term, args.IdLeader)
		nr.currentTerm = args.Term
		nr.votedFor = IntNOINICIALIZADO
		nr.role = Follower
		nr.IdLider = args.IdLeader
	} else {
		// args.Term == nr.currentTerm: actualizamos IdLider (quien nos envía AE)
		// pero NO forzamos a follower si ya somos leader en este término.
		nr.IdLider = args.IdLeader
	}

	// Reset election timer (valid heartbeat)
	nr.electionReset = time.Now()
	nr.resetTimer()

	// Consistency check
	if args.PrevLogIndex > nr.lastLogIndex() ||
		(args.PrevLogIndex > 0 && nr.logEntries[args.PrevLogIndex].Term != args.PrevLogTerm) {
		// nr.Logger.Printf("AE X Nodo %d inconsistencia de log con líder %d: PrevLogIndex=%d PrevLogTerm=%d (len=%d)",
		// 	nr.Yo, args.IdLeader, args.PrevLogIndex, args.PrevLogTerm, len(nr.logEntries))
		return nil
	}

	// Append entries if any (truncate conflicting entries first)
	if len(args.Entries) > 0 {
		nr.logEntries = nr.logEntries[:args.PrevLogIndex+1]
		nr.logEntries = append(nr.logEntries, args.Entries...)
		// nr.Logger.Printf("AE + Nodo %d anexa %d entradas nuevas del líder %d (len=%d)",
		// 	nr.Yo, len(args.Entries), args.IdLeader, len(nr.logEntries))
	} else {
		// nr.Logger.Printf("AE • Nodo %d recibe latido vacío (heartbeat) de %d", nr.Yo, args.IdLeader)
	}

	// Update commitIndex
	if args.LeaderCommit > nr.commitIndex {
		prev := nr.commitIndex
		nr.commitIndex = min(args.LeaderCommit, len(nr.logEntries)-1)
		if nr.commitIndex != prev {
			go nr.aplicarEntradas()
		}
	}

	results.Success = true
	results.Term = nr.currentTerm
	return nil
}

// enviarPeticionVoto: cliente RPC wrapper
func (nr *NodoRaft) enviarPeticionVoto(nodo int, args *ArgsPeticionVoto, reply *RespuestaPeticionVoto) bool {
	peer := nr.Nodos[nodo]
	err := peer.CallTimeout("NodoRaft.PedirVoto", args, reply, rpcTimeout*time.Millisecond)
	if err != nil {
		// nr.Logger.Printf("Nodo %d: fallo al pedir voto a %d (%v)", nr.Yo, nodo, err)
		return false
	}
	// nr.Logger.Printf("Nodo %d: respuesta voto de %d: Term=%d VoteGranted=%v", nr.Yo, nodo, reply.Term, reply.VoteGranted)
	return true
}

func (nr *NodoRaft) tratarRespuestaVoto(reply RespuestaPeticionVoto) {
	nr.Mux.Lock()
	defer nr.Mux.Unlock()

	if reply.Term > nr.currentTerm {
		nr.currentTerm = reply.Term
		nr.role = Follower
		nr.votedFor = IntNOINICIALIZADO
		nr.IdLider = -1
		nr.resetTimer()
		return
	}
	if nr.role != Candidate {
		return
	}
	if reply.VoteGranted && reply.Term == nr.currentTerm {
		nr.votesReceived++
		if nr.votesReceived > len(nr.Nodos)/2 {
			nr.role = Leader
			nr.IdLider = nr.Yo
			nr.votesReceived = 0
			// stop election timer
			if nr.timer != nil {
				nr.timer.Stop()
				nr.timer = nil
			}
			// init replication indices
			lastIdx := nr.lastLogIndex()
			for i := range nr.Nodos {
				nr.nextIndex[i] = lastIdx + 1
				nr.matchIndex[i] = 0
			}
			// start heartbeats
			go nr.enviarLatido()
			if nr.Logger != nil {
				nr.Logger.Printf("Nodo: %d Es elegido LIDER en mandato %d", nr.Yo, nr.currentTerm)
			}
		}
	}
}

// iniciarEleccion: lanzada por timer
func (nr *NodoRaft) iniciarEleccion() {
	nr.Mux.Lock()
	if nr.role == Leader {
		nr.Mux.Unlock()
		return
	}
	nr.currentTerm++
	nr.votedFor = nr.Yo
	nr.votesReceived = 1
	nr.role = Candidate
	nr.resetTimer()
	currentTerm := nr.currentTerm
	yo := nr.Yo
	nodos := append([]rpctimeout.HostPort(nil), nr.Nodos...)
	lastIdx := nr.lastLogIndex()
	lastTerm := nr.lastLogTerm()
	nr.Mux.Unlock()

	// nr.Logger.Printf("Nodo %d inicia elección para mandato %d", yo, currentTerm)

	args := ArgsPeticionVoto{
		Term:         currentTerm,
		CandidateId:  yo,
		LastLogIndex: lastIdx,
		LastLogTerm:  lastTerm,
	}
	for node := range nodos {
		if node == yo {
			continue
		}
		go func(node int) {
			reply := RespuestaPeticionVoto{}
			ok := nr.enviarPeticionVoto(node, &args, &reply)
			if ok {
				nr.tratarRespuestaVoto(reply)
			}
		}(node)
	}
}

// pushLoop: una goroutine por follower desde líder, gestiona heartbeats y replicación
func (nr *NodoRaft) pushLoop(node int, peer rpctimeout.HostPort) {
	heartbeatTicker := time.NewTicker(heartbeatRate * time.Millisecond)
	defer heartbeatTicker.Stop()

	for {
		// Fase 1: leer estado bajo lock y construir AE
		nr.Mux.Lock()
		if nr.role != Leader {
			nr.Mux.Unlock()
			return
		}
		term := nr.currentTerm
		commitIdx := nr.commitIndex
		nextIdx := nr.nextIndex[node]
		if nextIdx < 1 {
			nextIdx = 1
		}
		prevIdx := nextIdx - 1
		var prevTerm int
		if prevIdx == 0 {
			prevTerm = 0
		} else {
			prevTerm = nr.logEntries[prevIdx].Term
		}
		var entriesToSend []logEntry
		if nextIdx <= nr.lastLogIndex() {
			entriesToSend = append([]logEntry(nil), nr.logEntries[nextIdx:]...)
		}
		args := ArgAppendEntries{
			Term:         term,
			IdLeader:     nr.Yo,
			PrevLogIndex: prevIdx,
			PrevLogTerm:  prevTerm,
			Entries:      entriesToSend,
			LeaderCommit: commitIdx,
		}
		nr.Mux.Unlock()

		// Fase 2: RPC sin lock
		var reply Results
		err := peer.CallTimeout("NodoRaft.AppendEntries", &args, &reply, rpcTimeout*time.Millisecond)
		if err != nil {
			<-heartbeatTicker.C
			continue
		}

		// Fase 3: procesar respuesta bajo lock
		nr.Mux.Lock()
		if reply.Term > nr.currentTerm {
			nr.currentTerm = reply.Term
			nr.role = Follower
			nr.votedFor = IntNOINICIALIZADO
			nr.IdLider = -1
			nr.resetTimer()
			nr.Mux.Unlock()
			return
		}
		if nr.role != Leader {
			nr.Mux.Unlock()
			return
		}
		if reply.Success {
			if len(entriesToSend) > 0 {
				nr.matchIndex[node] = prevIdx + len(entriesToSend)
			} else {
				if nr.matchIndex[node] < prevIdx {
					nr.matchIndex[node] = prevIdx
				}
			}
			nr.nextIndex[node] = nr.matchIndex[node] + 1
			nr.electionReset = time.Now()
			nr.actualizarCommitLocked() // actualizar commit bajo lock
		} else {
			if nr.nextIndex[node] > 1 {
				nr.nextIndex[node]--
			}
		}
		nr.Mux.Unlock()

		<-heartbeatTicker.C
	}
}

// enviarLatido: crea pushLoop por cada follower (no bloquea)
func (nr *NodoRaft) enviarLatido() {
	nr.Mux.Lock()
	if nr.role != Leader {
		nr.Mux.Unlock()
		return
	}
	yo := nr.Yo
	peers := append([]rpctimeout.HostPort(nil), nr.Nodos...)
	nr.Mux.Unlock()

	// nr.Logger.Printf("Nodo %d comienza a enviar heartbeats y replicación", yo)
	for node, peer := range peers {
		if node == yo {
			continue
		}
		go nr.pushLoop(node, peer)
	}
}

// actualizarCommitLocked: asume que el caller tiene nr.Mux.Lock()
func (nr *NodoRaft) actualizarCommitLocked() {
	n := len(nr.logEntries) - 1
	for i := nr.commitIndex + 1; i <= n; i++ {
		cont := 1
		for node := range nr.Nodos {
			if node != nr.Yo && nr.matchIndex[node] >= i {
				cont++
			}
		}
		if cont > len(nr.Nodos)/2 && nr.logEntries[i].Term == nr.currentTerm {
			nr.commitIndex = i
			// al devolver el control al caller debe liberar Mutex
			// aplicarEntradas() lo vuelve a bloquear
			go nr.aplicarEntradas()
		}
	}
}

// Envía a la máquina de estados de cada nodo la operación
// siguiente a nr.lastApplied en el log
func (nr *NodoRaft) aplicarEntradas() {
	for {
		nr.Mux.Lock()
		if nr.lastApplied >= nr.commitIndex {
			nr.Mux.Unlock()
			return
		}
		nr.lastApplied++
		entry := nr.logEntries[nr.lastApplied]
		// Solo operaciones que añaden nuevos valores KV son actualizadas en logStore
		// para todos los nodos
		if entry.Operacion.Operacion == OP1 {
			nr.logStore[entry.Operacion.Clave] = entry.Operacion.Valor
		}
		nr.Mux.Unlock()

		op := AplicaOperacion{Indice: nr.lastApplied, Operacion: entry.Operacion}
		// enviar a canal sin bloquear mutex
		nr.canalAplicarOperacion <- op
		// nr.Logger.Printf("Nodo %d aplica operación %v en índice %d", nr.Yo, op.Operacion, op.Indice)
	}
}
