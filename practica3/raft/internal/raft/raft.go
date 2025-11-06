// Escribir vuestro código de funcionalidad Raft en este fichero
//
// Implementación base del algoritmo Raft siguiendo el documento adjunto (RaftDoc.pdf).
// Esta versión implementa: estados, temporizador de elección aleatorizado, elección de líder
// (esqueleto), latidos (heartbeats) como AppendEntries vacíos y manejo de términos.
//
// NOTA IMPORTANTE:
// - El transporte RPC exacto (rpctimeout) puede variar según vuestro entorno docente. He dejado
//   la llamada en enviarPeticionVoto y enviarAppendEntries como ejemplo. Si el nombre de método
//   o firma difiere, ajustad esas funciones.
// - La replicación de log completa (comprobaciones prevLogIndex/prevLogTerm, nextIndex/matchIndex,
//   compromiso de entradas y aplicación real al estado) está esbozada como TODO para que podáis
//   completarla por fases.
// - El temporizador de elección se inicializa de forma aleatoria en un intervalo fijo (p.ej. 150–300ms)
//   y se resetea cuando el nodo: (1) se convierte en candidato, (2) recibe AppendEntries del líder,
//   o (3) concede un voto.

package raft

import (
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"

	"raft/internal/comun/rpctimeout"
)

const (
	// Constante para fijar valor entero no inicializado
	IntNOINICIALIZADO = -1

	//  false deshabilita por completo los logs de depuracion
	// Aseguraros de poner kEnableDebugLogs a false antes de la entrega
	kEnableDebugLogs = true

	// Poner a true para logear a stdout en lugar de a fichero
	kLogToStdout = false

	// Cambiar esto para salida de logs en un directorio diferente
	kLogOutputDir = "./logs_raft/"

	// Intervalos de temporización (ajustables)
	electionMinMs     = 150 // recomendado 150–300ms
	electionMaxMs     = 300
	heartbeatInterval = 50 // latido del líder
	rpcTimeoutMs      = 80 // timeout por RPC (ajustad a vuestro entorno)
)

// Tipo de operación que expone la máquina de estados (clave/valor)
type TipoOperacion struct {
	Operacion string // "leer" | "escribir"
	Clave     string
	Valor     string // en lectura, Valor = ""
}

// Entrada de log de Raft (comando + término)
type logEntry struct {
	Term      int
	Operacion TipoOperacion
}

// Mensaje para aplicar una operación comprometida en la máquina de estados
type AplicaOperacion struct {
	Indice    int // índice del log
	Operacion TipoOperacion
}

// Tipo de dato Go que representa un solo nodo (réplica) de Raft
type NodoRaft struct {
	Mux sync.Mutex // Mutex para proteger acceso a estado compartido

	// Host:Port de todos los nodos (réplicas) Raft, en mismo orden
	Nodos   []rpctimeout.HostPort
	Yo      int         // indice de este nodo en el array "Nodos"
	IdLider int         // índice en Nodos del líder (o -1 si desconocido)
	Logger  *log.Logger // para facilitar depuración

	// Estado persistente en todos los nodos
	currentTerm int // último mandato visto por el nodo. 0 al inicializar.
	votedFor    int // candidato que recibió mi voto en mandato actual, o -1 si ninguno
	logEntries  []logEntry

	// Estado volátil (todos los servidores)
	commitIndex int // índice más alto comprometido (inicia 0)
	lastApplied int // índice más alto aplicado (inicia 0)

	// Estado volátil en líderes
	nextIndex  map[int]int // siguiente índice a enviar a cada seguidor
	matchIndex map[int]int // índice más alto replicado en cada seguidor

	// Estado temporal (no persistente)
	role                     int         // 0=follower, 1=candidate, 2=leader
	votesReceived            int         // votos recibidos durante la elección
	electionReset            time.Time   // última vez que se recibió mensaje válido / se reseteó la elección
	timer                    *time.Timer // para gestionar timeout de elecciones
	rng                      *rand.Rand
	initialElectionDelayUsed bool

	// Canal para aplicar operaciones comprometidas a la máquina de estados
	canalAplicarOperacion chan AplicaOperacion
}

// --- Utilidades de temporización ---
func (nr *NodoRaft) randomizedElectionTimeout() time.Duration {
	ms := electionMinMs + nr.rng.Intn(electionMaxMs-electionMinMs+1)
	return time.Duration(ms) * time.Millisecond
}

func (nr *NodoRaft) resetElectionTimerLocked() {
	var d time.Duration
	if !nr.initialElectionDelayUsed {
		d = 2500 * time.Millisecond // para que el T1 vea mandato=0 sin líder
		nr.initialElectionDelayUsed = true
	} else {
		d = nr.randomizedElectionTimeout() // 150–300 ms
	}

	if nr.timer == nil {
		nr.timer = time.NewTimer(d)
	} else {
		if !nr.timer.Stop() {
			select {
			case <-nr.timer.C:
			default:
			}
		}
		nr.timer.Reset(d)
	}
	nr.electionReset = time.Now()
}

// --- Creación de un nuevo nodo de elección ---
func NuevoNodo(nodos []rpctimeout.HostPort, yo int, canalAplicarOperacion chan AplicaOperacion) *NodoRaft {
	nr := &NodoRaft{}
	nr.Nodos = nodos
	nr.Yo = yo
	nr.IdLider = -1

	if kEnableDebugLogs {
		nombreNodo := nodos[yo].Host() + "_" + nodos[yo].Port()
		logPrefix := fmt.Sprintf("%s", nombreNodo)
		if kLogToStdout {
			nr.Logger = log.New(os.Stdout, nombreNodo+" -->> ", log.Lmicroseconds|log.Lshortfile)
		} else {
			if err := os.MkdirAll(kLogOutputDir, os.ModePerm); err != nil {
				panic(err.Error())
			}
			logOutputFile, err := os.OpenFile(fmt.Sprintf("%s/%s.txt", kLogOutputDir, logPrefix), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o755)
			if err != nil {
				panic(err.Error())
			}
			nr.Logger = log.New(logOutputFile, logPrefix+" -> ", log.Lmicroseconds|log.Lshortfile)
		}
		nr.Logger.Println("logger initialized")
	} else {
		nr.Logger = log.New(ioutil.Discard, "", 0)
	}

	// Estado inicial
	seed := time.Now().UnixNano() + int64(nr.Yo)*1000003
	nr.rng = rand.New(rand.NewSource(seed))
	nr.role = 0 // follower
	nr.votesReceived = 0
	nr.votedFor = -1
	nr.currentTerm = 0
	nr.commitIndex = 0
	nr.lastApplied = 0
	nr.initialElectionDelayUsed = false
	nr.logEntries = make([]logEntry, 1) // índice base 1: dejamos logEntries[0] vacío
	nr.nextIndex = make(map[int]int)
	nr.matchIndex = make(map[int]int)
	nr.canalAplicarOperacion = canalAplicarOperacion

	nr.resetElectionTimerLocked() // inicializa electionReset y el timer

	// Gorutina principal: gestiona timeout de elección y latidos
	go nr.run()
	go nr.runWatchdog()

	return nr
}

func (nr *NodoRaft) runWatchdog() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	deadline := 2500 * time.Millisecond
	lastOk := time.Now()
	for range ticker.C {
		nr.Mux.Lock()
		// Consideramos “OK” si soy líder o he recibido AE recientemente
		ok := nr.role == 2 || time.Since(nr.electionReset) < deadline
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

// Bucle principal del nodo: controla elecciones y latidos
func (nr *NodoRaft) run() {
	for {
		nr.Mux.Lock()
		role := nr.role
		t := nr.timer
		nr.Mux.Unlock()

		if role == 2 { // Líder: envía heartbeats periódicos
			nr.enviarHeartbeats()
			time.Sleep(heartbeatInterval * time.Millisecond)
			continue
		}

		select {
		case <-t.C:
			// Timeout de elección
			nr.startElection()
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func (nr *NodoRaft) startElection() {
	nr.Mux.Lock()
	nr.role = 1 // candidate
	nr.currentTerm++
	nr.votedFor = nr.Yo
	nr.votesReceived = 1 // voto para sí mismo
	nr.resetElectionTimerLocked()
	term := nr.currentTerm
	nr.Logger.Printf("[%d] Comienza elección en término %d\n", nr.Yo, term)
	nr.Mux.Unlock()

	// Enviar RequestVote a los demás
	var wg sync.WaitGroup
	for i := range nr.Nodos {
		if i == nr.Yo {
			continue
		}
		wg.Add(1)
		go func(peer int) {
			defer wg.Done()
			args := &ArgsPeticionVoto{
				Term:         term,
				CandidateId:  nr.Yo,
				LastLogIndex: nr.lastLogIndex(),
				LastLogTerm:  nr.lastLogTerm(),
			}
			var reply RespuestaPeticionVoto
			ok := nr.enviarPeticionVoto(peer, args, &reply)
			if !ok {
				return
			}

			nr.Mux.Lock()
			defer nr.Mux.Unlock()
			if reply.Term > nr.currentTerm {
				nr.becomeFollowerLocked(reply.Term, -1)
				return
			}
			if nr.role == 1 && reply.VoteGranted && reply.Term == term {
				nr.votesReceived++
				if nr.hasMajority(nr.votesReceived) {
					nr.becomeLeaderLocked()
				}
			}
		}(i)
	}
	wg.Wait()
}

func (nr *NodoRaft) hasMajority(votes int) bool {
	return votes > len(nr.Nodos)/2
}

func (nr *NodoRaft) becomeFollowerLocked(newTerm int, leader int) {
	nr.role = 0
	nr.currentTerm = newTerm
	nr.votedFor = -1
	nr.IdLider = leader
	nr.votesReceived = 0
	nr.resetElectionTimerLocked()
	nr.Logger.Printf("[%d] Paso a follower, término=%d, líder=%d\n", nr.Yo, nr.currentTerm, nr.IdLider)
}

func (nr *NodoRaft) becomeLeaderLocked() {
	nr.role = 2
	nr.IdLider = nr.Yo
	// Inicializa nextIndex/matchIndex
	lastIdx := nr.lastLogIndex()
	for i := range nr.Nodos {
		if i == nr.Yo {
			continue
		}
		nr.nextIndex[i] = lastIdx + 1
		nr.matchIndex[i] = 0
	}
	nr.Logger.Printf("[%d] *** Soy líder en término %d ***\n", nr.Yo, nr.currentTerm)
}

// --- Estado/Helpers de log ---
func (nr *NodoRaft) lastLogIndex() int {
	return len(nr.logEntries) - 1 // porque logEntries[0] es hueco
}

func (nr *NodoRaft) lastLogTerm() int {
	if len(nr.logEntries) == 1 {
		return 0
	}
	return nr.logEntries[len(nr.logEntries)-1].Term
}

// ===== Helpers de commit y replicación (Práctica 3) =====
// En esta práctica NO aplicamos a la máquina de estados (eso será en la 5).
// Aun así, avanzamos commitIndex cuando exista mayoría.
func (nr *NodoRaft) tryAdvanceCommitIndexLocked() {
	if nr.role != 2 { // solo líder
		return
	}
	N := nr.commitIndex + 1
	for N <= nr.lastLogIndex() {
		cont := 1 // el líder cuenta
		for i := range nr.Nodos {
			if i == nr.Yo {
				continue
			}
			if nr.matchIndex[i] >= N {
				cont++
			}
		}
		// En la práctica 3 no exigen la regla del término actual explícitamente (§5.4.1 es para la 4),
		// pero la mantenemos para seguridad: solo avanzar entradas de este término
		if cont > len(nr.Nodos)/2 && nr.logEntries[N].Term == nr.currentTerm {
			nr.commitIndex = N
			N++
		} else {
			break
		}
	}
	// APLICAR entradas comprometidas recién alcanzadas
	nr.applyCommittedEntriesLocked()
}

// Aplica en la máquina de estados (vía canalAplicarOperacion) todas las
// entradas comprometidas que aún no han sido aplicadas.
// Debe llamarse SIEMPRE con el mutex tomado.
func (nr *NodoRaft) applyCommittedEntriesLocked() {
	if nr.lastApplied >= nr.commitIndex {
		return
	}

	start := nr.lastApplied + 1
	end := nr.commitIndex

	// Preparamos fuera del candado qué operaciones enviar
	applies := make([]AplicaOperacion, 0, end-start+1)
	for i := start; i <= end; i++ {
		applies = append(applies, AplicaOperacion{
			Indice:    i,
			Operacion: nr.logEntries[i].Operacion,
		})
	}
	nr.lastApplied = end

	// Enviar por el canal fuera de la sección crítica

	go func(ops []AplicaOperacion) {
		for _, op := range ops {
			nr.Logger.Printf("Nodo %d Mando aplicar %v", nr.Yo, op)
			nr.canalAplicarOperacion <- op
		}
	}(applies)
}

// Empuja log a un peer: maneja conflictos (retrocede nextIndex) y reintenta
func (nr *NodoRaft) pushLogToPeer(peer int) {
	for {
		// Captura consistente bajo cerrojo
		nr.Mux.Lock()
		if nr.role != 2 {
			nr.Mux.Unlock()
			return
		}
		next := nr.nextIndex[peer]
		prevIdx := next - 1
		prevTerm := 0
		if prevIdx > 0 && prevIdx < len(nr.logEntries) {
			prevTerm = nr.logEntries[prevIdx].Term
		}
		entries := make([]logEntry, 0)
		if next <= nr.lastLogIndex() {
			entries = append(entries, nr.logEntries[next:]...)
		}
		args := &ArgAppendEntries{
			Term:         nr.currentTerm,
			LeaderId:     nr.Yo,
			PrevLogIndex: prevIdx,
			PrevLogTerm:  prevTerm,
			Entries:      entries,
			LeaderCommit: nr.commitIndex,
		}
		nr.Mux.Unlock()

		var reply Results
		ok := nr.enviarAppendEntries(peer, args, &reply)
		if !ok {
			return // reintentaremos con el próximo latido
		}

		nr.Mux.Lock()
		if reply.Term > nr.currentTerm { // ver término mayor ⇒ follower
			nr.becomeFollowerLocked(reply.Term, -1)
			nr.Mux.Unlock()
			return
		}
		if nr.role != 2 {
			nr.Mux.Unlock()
			return
		}
		if reply.Success {
			if len(entries) > 0 {
				nr.matchIndex[peer] = args.PrevLogIndex + len(entries)
				nr.nextIndex[peer] = nr.matchIndex[peer] + 1
			}
			// Intentar avanzar commit con la nueva mayoría (y aplicar)
			nr.tryAdvanceCommitIndexLocked()
			nr.Mux.Unlock()
			return
		}
		// Conflicto ⇒ retroceder nextIndex y reintentar
		if nr.nextIndex[peer] > 1 {
			nr.nextIndex[peer]--
			nr.Mux.Unlock()
			continue
		}
		nr.Mux.Unlock()
		return
	}
}

// --- API público del nodo ---
func (nr *NodoRaft) para() {
	go func() { time.Sleep(5 * time.Millisecond); os.Exit(0) }()
}

func (nr *NodoRaft) obtenerEstado() (int, int, bool, int) {
	nr.Mux.Lock()
	defer nr.Mux.Unlock()
	return nr.Yo, nr.currentTerm, nr.role == 2, nr.IdLider
}

func (nr *NodoRaft) someterOperacion(operacion TipoOperacion) (int, int, bool, int, string) {
	nr.Mux.Lock()
	if nr.role != 2 {
		defer nr.Mux.Unlock()
		return -1, nr.currentTerm, false, nr.IdLider, ""
	}
	// 1) Añadir entrada local
	entry := logEntry{Term: nr.currentTerm, Operacion: operacion}
	nr.logEntries = append(nr.logEntries, entry)
	index := nr.lastLogIndex()
	term := nr.currentTerm
	leaderId := nr.IdLider

	// Snapshot de peers
	peers := make([]int, 0, len(nr.Nodos)-1)
	for i := range nr.Nodos {
		if i == nr.Yo {
			continue
		}
		peers = append(peers, i)
	}
	nr.Mux.Unlock()

	// 2) Empujar a seguidores (best-effort)
	var wg sync.WaitGroup
	for _, p := range peers {
		wg.Add(1)
		go func(peer int) {
			defer wg.Done()
			nr.pushLogToPeer(peer)
		}(p)
	}
	wg.Wait()

	// 3) Intentar avanzar commit por mayoría (y aplicar)
	nr.Mux.Lock()
	nr.tryAdvanceCommitIndexLocked()
	nr.Mux.Unlock()

	return index, term, true, leaderId, ""
}

// -----------------------------------------------------------------------
// LLAMADAS RPC al API
// -----------------------------------------------------------------------

type Vacio struct{}

func (nr *NodoRaft) ParaNodo(args Vacio, reply *Vacio) error {
	defer nr.para()
	return nil
}

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
	reply.IdNodo, reply.Mandato, reply.EsLider, reply.IdLider = nr.obtenerEstado()
	return nil
}

type ResultadoRemoto struct {
	ValorADevolver string
	IndiceRegistro int
	EstadoParcial
}

func (nr *NodoRaft) SometerOperacionRaft(operacion TipoOperacion, reply *ResultadoRemoto) error {
	reply.IndiceRegistro, reply.Mandato, reply.EsLider, reply.IdLider, reply.ValorADevolver = nr.someterOperacion(operacion)
	return nil
}

// -----------------------------------------------------------------------
// LLAMADAS RPC protocolo RAFT
// -----------------------------------------------------------------------

// Args y Reply de Petición de Voto (RequestVote)
type ArgsPeticionVoto struct {
	Term         int // término del candidato
	CandidateId  int // id del candidato (índice en Nodos)
	LastLogIndex int // índice de la última entrada del candidato
	LastLogTerm  int // término de la última entrada del candidato
}

type RespuestaPeticionVoto struct {
	Term        int  // término actual para actualizar al candidato
	VoteGranted bool // true si se otorga el voto
}

// Reglas de voto mínimas: a) término no atrás, b) no he votado en este término o he votado por él,
// c) (opcional/seguridad) su log está al menos tan actualizado como el mío (LastLogTerm/LastLogIndex)
func (nr *NodoRaft) PedirVoto(peticion *ArgsPeticionVoto, reply *RespuestaPeticionVoto) error {
	nr.Mux.Lock()
	defer nr.Mux.Unlock()

	reply.VoteGranted = false
	reply.Term = nr.currentTerm

	if peticion.Term < nr.currentTerm {
		return nil
	}
	if peticion.Term > nr.currentTerm {
		nr.becomeFollowerLocked(peticion.Term, peticion.CandidateId)
		reply.Term = nr.currentTerm
	}

	upToDate := false
	if peticion.LastLogTerm > nr.lastLogTerm() {
		upToDate = true
	} else if peticion.LastLogTerm == nr.lastLogTerm() && peticion.LastLogIndex >= nr.lastLogIndex() {
		upToDate = true
	}

	if (nr.votedFor == -1 || nr.votedFor == peticion.CandidateId) && upToDate {
		nr.votedFor = peticion.CandidateId
		reply.VoteGranted = true
		// Reset del temporizador de elección al conceder voto
		nr.resetElectionTimerLocked()
	}
	return nil
}

// AppendEntries (usado como latido si Entries está vacío)
type ArgAppendEntries struct {
	Term         int        // término del líder
	LeaderId     int        // id del líder
	PrevLogIndex int        // índice previo a las nuevas entradas
	PrevLogTerm  int        // término de PrevLogIndex
	Entries      []logEntry // entradas a guardar (vacías para heartbeat)
	LeaderCommit int        // índice commit del líder
}

type Results struct {
	Term    int  // término actual del seguidor
	Success bool // true si el seguidor contiene entrada previa que coincide
}

func (nr *NodoRaft) AppendEntries(args *ArgAppendEntries, results *Results) error {
	nr.Mux.Lock()
	defer nr.Mux.Unlock()

	results.Success = false
	results.Term = nr.currentTerm

	// 1) Terminos
	if args.Term < nr.currentTerm {
		return nil
	}
	// Vemos un término >=: nos alineamos como follower y reconocemos líder
	if args.Term > nr.currentTerm || nr.role != 0 {
		nr.becomeFollowerLocked(args.Term, args.LeaderId)
	} else {
		nr.IdLider = args.LeaderId
	}

	// 2) Heartbeat/replicación ⇒ resetear timeout
	nr.resetElectionTimerLocked()

	// 3) Comprobación de coherencia (PrevLogIndex, PrevLogTerm)
	if args.PrevLogIndex > nr.lastLogIndex() {
		return nil // aún no tengo esa entrada
	}
	if args.PrevLogIndex > 0 && nr.logEntries[args.PrevLogIndex].Term != args.PrevLogTerm {
		return nil // conflicto en PrevLog
	}

	// 4) Borrar conflictos solapados y anexar nuevas entradas
	insertAt := args.PrevLogIndex + 1
	i := 0
	for insertAt+i <= nr.lastLogIndex() && i < len(args.Entries) {
		if nr.logEntries[insertAt+i].Term != args.Entries[i].Term {
			// truncar desde aquí
			nr.logEntries = nr.logEntries[:insertAt+i]
			break
		}
		i++
	}
	for ; i < len(args.Entries); i++ {
		nr.logEntries = append(nr.logEntries, args.Entries[i])
	}

	// 5) Actualizar commitIndex (y aplicar a SM)
	if args.LeaderCommit > nr.commitIndex {
		lastNew := nr.lastLogIndex()
		if args.LeaderCommit < lastNew {
			nr.commitIndex = args.LeaderCommit
		} else {
			nr.commitIndex = lastNew
		}
		// aplicar lo que acaba de quedar comprometido
		nr.applyCommittedEntriesLocked()
	}

	results.Success = true
	results.Term = nr.currentTerm
	return nil
}

// ----- Métodos/Funciones a utilizar como clientes -----

// Enviar Petición de Voto a un peer
func (nr *NodoRaft) enviarPeticionVoto(nodo int, args *ArgsPeticionVoto, reply *RespuestaPeticionVoto) bool {
	// El método CallTimeout devuelve error; éxito si err == nil
	err := nr.Nodos[nodo].CallTimeout("NodoRaft.PedirVoto", args, reply, time.Duration(rpcTimeoutMs)*time.Millisecond)
	if err != nil {
		if kEnableDebugLogs {
			nr.Logger.Printf("[%d] RPC PedirVoto a %d ERROR: %v", nr.Yo, nodo, err)
		}
		return false
	}
	return true
}

// Enviar AppendEntries (latido) a un peer
func (nr *NodoRaft) enviarAppendEntries(nodo int, args *ArgAppendEntries, reply *Results) bool {
	err := nr.Nodos[nodo].CallTimeout("NodoRaft.AppendEntries", args, reply, time.Duration(rpcTimeoutMs)*time.Millisecond)
	if err != nil {
		if kEnableDebugLogs {
			nr.Logger.Printf("[%d] RPC AppendEntries a %d ERROR: %v", nr.Yo, nodo, err)
		}
		return false
	}
	return true
}

// Envío periódico de latidos por el líder
func (nr *NodoRaft) enviarHeartbeats() {
	nr.Mux.Lock()
	if nr.role != 2 {
		nr.Mux.Unlock()
		return
	}
	leaderId := nr.Yo
	nr.Mux.Unlock()

	var wg sync.WaitGroup
	for i := range nr.Nodos {
		if i == leaderId {
			continue
		}
		wg.Add(1)
		go func(peer int) {
			defer wg.Done()
			nr.pushLogToPeer(peer) // envía entradas pendientes o heartbeat
		}(i)
	}
	wg.Wait()

}
