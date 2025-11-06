// Escribir vuestro código de funcionalidad Raft en este fichero
//

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

	//"crypto/rand"
	"sync"
	"time"

	//"net/rpc"

	"raft/internal/comun/rpctimeout"
)

// para representar los estados de un servidor
type state int

const (
	// Constante para fijar valor entero no inicializado
	IntNOINICIALIZADO = -1

	//  false deshabilita por completo los logs de depuracion
	// Aseguraros de poner kEnableDebugLogs a false antes de la entrega
	kEnableDebugLogs = true

	// Poner a true para logear a stdout en lugar de a fichero
	kLogToStdout = true

	// Cambiar esto para salida de logs en un directorio diferente
	kLogOutputDir = "./logs_raft/"
	// base y techo del timer para nueva elección (ms)
	baseTimer = 220 // ≥ 3*heartbeat + margen
	ceilTimer = 520 // jitter suficiente
	// Timeout de la llamada RPC en ms (ajustar al entorno donde se ejecuta)
	rpcTimeout = 100
	// tiempo entre latidos enviados por el líder (ms)
	heartbeatRate = 60
	// tiempo máximo para abortar si no se consigue consenso (ms)
	Deadline = 2500
	// definición de estados del servidor
	Follower state = iota
	Candidate
	Leader
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

// Tipo de dato Go que representa un solo nodo (réplica) de raft
type NodoRaft struct {
	Mux sync.Mutex // Mutex para proteger acceso a estado compartido

	// Host:Port de todos los nodos (réplicas) Raft, en mismo orden
	Nodos   []rpctimeout.HostPort
	Yo      int         // indice de este nodos en campo array "nodos"
	IdLider int         // índice en Nodos del líder (o -1 si desconocido)
	Logger  *log.Logger // para facilitar depuración

	//

	// Estado persistente en todos los nodos
	currentTerm int        // último mandato visto por el nodo. 0 al inicializar.
	votedFor    int        // candidato que recibió mi voto en mandato actual o -1 si ninguno
	logEntries  []logEntry // registro de entradas en máquina estados

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
	role                     state       // "Follower", "Candidate", "Leader"
	votesReceived            int         // votos durante elección
	electionReset            time.Time   // última vez que recibió mensaje válido
	timer                    *time.Timer // para gestionar timeout de elecciones
	rng                      *rand.Rand  // para usar una semilla diferente para cada nodo
	initialElectionDelayUsed bool        // retardo de primera elección mayor
	// Canal para aplicar operaciones comprometidas a la máquina de estados
	canalAplicarOperacion chan AplicaOperacion
}

// PRE: nr.Mux debe estar bloqueado por quien realiza la llamada
// POST:
// resetTimer reinicia el temporizador de elección del nodo.
// Cancela el temporizador anterior (si existe) y programa uno nuevo
// con una duración aleatoria en el rango [baseTimer, ceilTimer].
// la primera vez que se llama inicia con un timer entre 1000 y 1500 ms
func (nr *NodoRaft) resetTimer() {
	var dur time.Duration
	if !nr.initialElectionDelayUsed { // primera vez el retardo es mayor: espera resto de nodos
		nr.initialElectionDelayUsed = true
		dur = time.Duration(1500+nr.rng.Intn(500)) * time.Millisecond
	} else {
		// Duración aleatoria para evitar colisiones simultáneas de elección
		dur = time.Duration(baseTimer+nr.rng.Intn(ceilTimer-baseTimer)) * time.Millisecond
	}
	// para evitar condiciones de carrera solo lanzamos AfterFunc() si no hay timer
	if nr.timer == nil {
		nr.timer = time.AfterFunc(dur, nr.iniciarEleccion)
	} else {
		nr.timer.Reset(dur)
	}
	// nr.Logger.Printf("[Nodo %d] Temporizador reiniciado (timeout: %v)", nr.Yo, dur)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Creacion de un nuevo nodo de eleccion
//
// Tabla de <Direccion IP:puerto> de cada nodo incluido a si mismo.
//
// <Direccion IP:puerto> de este nodo esta en nodos[yo]
//
// Todos los arrays nodos[] de los nodos tienen el mismo orden

// canalAplicar es un canal donde, en la practica 5, se recogerán las
// operaciones a aplicar a la máquina de estados. Se puede asumir que
// este canal se consumira de forma continúa.
//
// NuevoNodo() debe devolver resultado rápido, por lo que se deberían
// poner en marcha Gorutinas para trabajos de larga duracion
func NuevoNodo(nodos []rpctimeout.HostPort, yo int,
	canalAplicarOperacion chan AplicaOperacion) *NodoRaft {
	nr := &NodoRaft{}
	nr.Nodos = nodos
	nr.Yo = yo
	nr.IdLider = -1

	if kEnableDebugLogs {
		nombreNodo := nodos[yo].Host() + "_" + nodos[yo].Port()
		logPrefix := fmt.Sprintf("%s", nombreNodo)

		fmt.Println("LogPrefix: ", logPrefix)

		if kLogToStdout {
			nr.Logger = log.New(os.Stdout, nombreNodo+" -->> ",
				log.Lmicroseconds|log.Lshortfile)
		} else {
			err := os.MkdirAll(kLogOutputDir, os.ModePerm)
			if err != nil {
				panic(err.Error())
			}
			logOutputFile, err := os.OpenFile(fmt.Sprintf("%s/%s.txt",
				kLogOutputDir, logPrefix), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
			if err != nil {
				panic(err.Error())
			}
			nr.Logger = log.New(logOutputFile,
				logPrefix+" -> ", log.Lmicroseconds|log.Lshortfile)
		}
		nr.Logger.Println("logger initialized")
	} else {
		nr.Logger = log.New(io.Discard, "", 0)
	}
	// prepara estado inicial
	nr.rng = rand.New(rand.NewSource(time.Now().UnixNano() + int64(nr.Yo)*100005))
	nr.currentTerm = 0
	nr.votedFor = IntNOINICIALIZADO
	nr.logEntries = make([]logEntry, 1) // índice base 1: dejamos logEntries[0] vacío

	nr.commitIndex = 0
	nr.lastApplied = 0

	nr.nextIndex = make(map[int]int, len(nr.Nodos))
	nr.matchIndex = make(map[int]int, len(nr.Nodos))
	// Inicializa índices (todos empiezan apuntando al final del log vacío)
	lastIdx := len(nr.logEntries) - 1
	for i := range nodos {
		nr.nextIndex[i] = lastIdx + 1
		nr.matchIndex[i] = 0
	}

	nr.role = Follower
	nr.votesReceived = 0
	nr.initialElectionDelayUsed = false
	nr.electionReset = time.Now()
	nr.resetTimer()
	go nr.runWatchdog()
	return nr
}

func (nr *NodoRaft) runWatchdog() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	deadline := Deadline * time.Millisecond
	lastOk := time.Now()
	for range ticker.C {
		nr.Mux.Lock()
		// Consideramos “OK” si soy líder o he recibido AE recientemente
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

// Metodo Para() utilizado cuando no se necesita mas al nodo
//
// Quizas interesante desactivar la salida de depuracion
// de este nodo
func (nr *NodoRaft) para() {
	go func() { time.Sleep(5 * time.Millisecond); os.Exit(0) }()
}

// Devuelve "yo", mandato en curso y si este nodo cree ser lider
//
// Primer valor devuelto es el indice de este  nodo Raft el el conjunto de nodos
// la operacion si consigue comprometerse.
// El segundo valor es el mandato en curso
// El tercer valor es true si el nodo cree ser el lider
// Cuarto valor es el lider, es el indice del líder si no es él
func (nr *NodoRaft) obtenerEstado() (int, int, bool, int) {
	nr.Mux.Lock()
	var yo int = nr.Yo
	var mandato int = nr.currentTerm
	var esLider bool = nr.Yo == nr.IdLider
	var idLider int = nr.IdLider
	nr.Mux.Unlock()

	return yo, mandato, esLider, idLider
}

// El servicio que utilice Raft (base de datos clave/valor, por ejemplo)
// Quiere buscar un acuerdo de posicion en registro para siguiente operacion
// solicitada por cliente.

// Si el nodo no es el lider, devolver falso
// Sino, comenzar la operacion de consenso sobre la operacion y devolver en
// cuanto se consiga
//
// No hay garantia que esta operacion consiga comprometerse en una entrada de
// de registro, dado que el lider puede fallar y la entrada ser reemplazada
// en el futuro.
// Primer valor devuelto es el indice del registro donde se va a colocar
// la operacion si consigue comprometerse.
// El segundo valor es el mandato en curso
// El tercer valor es true si el nodo cree ser el lider
// Cuarto valor es el lider, es el indice del líder si no es él
func (nr *NodoRaft) someterOperacion(operacion TipoOperacion) (int, int,
	bool, int, string) {
	indice := -1
	mandato := -1
	EsLider := false
	idLider := -1
	valorADevolver := ""

	nr.Mux.Lock()
	defer nr.Mux.Unlock()

	mandato = nr.currentTerm
	idLider = nr.IdLider
	if nr.role != Leader {
		return -1, mandato, false, idLider, ""
	}
	EsLider = true
	// Añadir la entrada localmente
	entry := logEntry{Term: nr.currentTerm, Operacion: operacion}
	nr.logEntries = append(nr.logEntries, entry)
	indice = len(nr.logEntries) - 1

	// Lanzamos replicación a los seguidores
	go nr.replicarEntradas()

	return indice, mandato, EsLider, idLider, valorADevolver
}

func (nr *NodoRaft) replicarEntradas() {
	nr.Mux.Lock()
	if nr.role != Leader {
		nr.Mux.Unlock()
		return
	}
	currentTerm := nr.currentTerm
	nr.Mux.Unlock()

	for node, peer := range nr.Nodos {
		if node == nr.Yo {
			continue
		}
		go func(peer rpctimeout.HostPort, node int) {
			for {
				nr.Mux.Lock()
				if nr.role != Leader {
					nr.Mux.Unlock()
					return
				}
				// nextIndex puede estar sin inicializar (0). Forzamos base 1.
				nextIdx := nr.nextIndex[node]
				if nextIdx < 1 {
					nextIdx = 1
				}
				prevIdx := nextIdx - 1

				// Con tu diseño: logEntries[0] existe y su Term implícito es 0.
				// Si prevIdx==0, prevTerm=0; si no, toma el término real.
				var prevTerm int
				if prevIdx == 0 {
					prevTerm = 0
				} else {
					prevTerm = nr.logEntries[prevIdx].Term
				}

				args := ArgAppendEntries{
					Term:         currentTerm,
					IdLeader:     nr.Yo,
					PrevLogIndex: prevIdx,
					PrevLogTerm:  prevTerm,
					Entries:      append([]logEntry(nil), nr.logEntries[nextIdx:]...),
					LeaderCommit: nr.commitIndex,
				}
				nr.Mux.Unlock()

				reply := Results{}
				err := peer.CallTimeout("NodoRaft.AppendEntries", &args, &reply, rpcTimeout*time.Millisecond)
				if err != nil {
					time.Sleep(50 * time.Millisecond)
					continue
				}

				nr.Mux.Lock()
				if reply.Term > nr.currentTerm {
					nr.role = Follower
					nr.currentTerm = reply.Term
					nr.votedFor = IntNOINICIALIZADO
					nr.IdLider = IntNOINICIALIZADO
					nr.resetTimer()
					nr.Mux.Unlock()
					return
				}
				if reply.Success {
					nr.matchIndex[node] = prevIdx + len(args.Entries)
					nr.nextIndex[node] = nr.matchIndex[node] + 1
					nr.Mux.Unlock()
					nr.actualizarCommit()
					return
				} else {
					// Decrementar nextIndex para retroceder y reintentar
					if nr.nextIndex[node] > 1 {
						nr.nextIndex[node]--
					}
					nr.Mux.Unlock()
					time.Sleep(50 * time.Millisecond)
				}
			}
		}(peer, node)
	}
}

func (nr *NodoRaft) actualizarCommit() {
	n := len(nr.logEntries) - 1
	for i := nr.commitIndex + 1; i <= n; i++ {
		cont := 1 // el líder se cuenta a sí mismo
		for node := range nr.Nodos {
			if node == nr.Yo {
				continue
			}
			if nr.matchIndex[node] >= i {
				cont++
			}
		}
		if cont > len(nr.Nodos)/2 && nr.logEntries[i].Term == nr.currentTerm {
			nr.commitIndex = i
			go nr.aplicarEntradas()
		}
	}
}

func (nr *NodoRaft) aplicarEntradas() {
	nr.Mux.Lock()
	defer nr.Mux.Unlock()

	for nr.lastApplied < nr.commitIndex {
		nr.lastApplied++
		entry := nr.logEntries[nr.lastApplied]
		op := AplicaOperacion{
			Indice:    nr.lastApplied,
			Operacion: entry.Operacion,
		}
		nr.canalAplicarOperacion <- op
		nr.Logger.Printf("Nodo %d aplica operación %v en índice %d", nr.Yo, op.Operacion, op.Indice)
	}
}

func (nr *NodoRaft) iniciarEleccion() {
	nr.Mux.Lock()
	if nr.role == Leader { // solo los líderes no lanzan nueva elección
		nr.Mux.Unlock()
		return
	}

	// incrementa mandato para esta nueva elección
	nr.currentTerm++
	nr.votedFor = nr.Yo
	nr.votesReceived = 1 // voto por mí mismo
	nr.role = Candidate
	nr.resetTimer() // para evitar que vuelva a dispararse elección
	// copia de los valores que necesito antes de liberar Mutex
	currentTerm := nr.currentTerm
	yo := nr.Yo
	nodos := append([]rpctimeout.HostPort(nil), nr.Nodos...)
	nr.Mux.Unlock()
	nr.Logger.Printf("Nodo %d inicia elección para mandato %d\n", nr.Yo, nr.currentTerm)

	args := ArgsPeticionVoto{
		Term:         currentTerm,
		CandidateId:  yo,
		LastLogIndex: nr.lastLogIndex(),
		LastLogTerm:  nr.lastLogTerm(),
	}
	for node, peer := range nodos {
		if node == yo {
			continue
		}
		go func(peer rpctimeout.HostPort, node int) {
			reply := RespuestaPeticionVoto{}
			ok := nr.enviarPeticionVoto(node, &args, &reply)
			if ok {
				nr.tratarRespuestaVoto(reply)
			}
		}(peer, node)
	}
}

// -----------------------------------------------------------------------
// LLAMADAS RPC al API
//
// Si no tenemos argumentos o respuesta estructura vacia (tamaño cero)
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

func (nr *NodoRaft) SometerOperacionRaft(operacion TipoOperacion,
	reply *ResultadoRemoto) error {
	reply.IndiceRegistro, reply.Mandato, reply.EsLider,
		reply.IdLider, reply.ValorADevolver = nr.someterOperacion(operacion)
	return nil
}

// -----------------------------------------------------------------------
// LLAMADAS RPC protocolo RAFT
//
// Structura de ejemplo de argumentos de RPC PedirVoto.
//
// Recordar
// -----------
// Nombres de campos deben comenzar con letra mayuscula !
type ArgsPeticionVoto struct {
	Term         int // mandato al que se presenta como candidato
	CandidateId  int // nº nodo en Nodos de quien se presenta como candidato
	LastLogIndex int // índice del último log del candidato
	LastLogTerm  int // mandato en el que se produjo el último commit del candidato
}

// Structura de ejemplo de respuesta de RPC PedirVoto,
//
// Recordar
// -----------
// Nombres de campos deben comenzar con letra mayuscula !
type RespuestaPeticionVoto struct {
	Term        int  // mandato del seguidor que responde
	VoteGranted bool //true si el voto fue concedido
}

// Metodo para RPC PedirVoto
func (nr *NodoRaft) PedirVoto(peticion *ArgsPeticionVoto,
	reply *RespuestaPeticionVoto) error {
	nr.Mux.Lock()
	defer nr.Mux.Unlock()

	reply.Term = nr.currentTerm
	reply.VoteGranted = false // en principio mi voto es negativo

	// si el mandato del candidato es menor al mío, emito voto negativo
	// también le informo que estamos en un mandato superior
	if peticion.Term < nr.currentTerm {
		nr.Logger.Printf("Nodo: %d Rechaza voto para nodo %d porque está mandato menor->(%d < %d)",
			nr.Yo, peticion.CandidateId, peticion.Term, nr.currentTerm)
		return nil
	}
	// si el mandato del candidato es más actual que el mío, me actualizo y le voto
	if peticion.Term > nr.currentTerm {
		nr.currentTerm = peticion.Term
		nr.votedFor = IntNOINICIALIZADO
		nr.role = Follower
	}
	// Regla de up-to-date
	if !(nr.votedFor == IntNOINICIALIZADO || nr.votedFor == peticion.CandidateId) ||
		!nr.isCandidateUpToDate(peticion.LastLogIndex, peticion.LastLogTerm) {
		// no vota
		reply.Term = nr.currentTerm
		return nil
	}
	nr.votedFor = peticion.CandidateId
	reply.VoteGranted = true
	nr.resetTimer()
	reply.Term = nr.currentTerm
	nr.Logger.Printf("Nodo: %d Vota por %d en mandato %d",
		nr.Yo, nr.votedFor, nr.currentTerm)
	return nil
}

// AppendEntries (usado como latido si Entries está vacío)
type ArgAppendEntries struct {
	Term         int        // término del líder
	IdLeader     int        // id del líder
	PrevLogIndex int        // índice previo a las nuevas entradas
	PrevLogTerm  int        // término de PrevLogIndex
	Entries      []logEntry // entradas a guardar (vacías para heartbeat)
	LeaderCommit int        // índice commit del líder
}

type Results struct {
	Term    int  // mandato actual del follower
	Success bool // true si el follower acepta la entrada (o latido)
}

// Metodo de tratamiento de llamadas RPC AppendEntries
func (nr *NodoRaft) AppendEntries(args *ArgAppendEntries, results *Results) error {
	nr.Mux.Lock()
	defer nr.Mux.Unlock()

	results.Success = false
	results.Term = nr.currentTerm

	// 1 Si el líder remoto tiene mandato menor, rechazamos
	if args.Term < nr.currentTerm {
		nr.Logger.Printf("Nodo %d rechaza AppendEntries de %d (mandato %d < %d)",
			nr.Yo, args.IdLeader, args.Term, nr.currentTerm)
		return nil
	}

	// 2️ Si llega un mandato mayor, me actualizo
	if args.Term > nr.currentTerm {
		nr.currentTerm = args.Term
		nr.votedFor = IntNOINICIALIZADO
		nr.role = Follower
		nr.IdLider = args.IdLeader
	}

	// 4️ Es un AppendEntries válido desde el líder actual o igual mandato
	nr.electionReset = time.Now()
	nr.resetTimer()
	nr.role = Follower
	nr.IdLider = args.IdLeader

	// 5️ Comprobar coherencia del log
	if args.PrevLogIndex > nr.lastLogIndex() ||
		(args.PrevLogIndex > 0 && nr.logEntries[args.PrevLogIndex].Term != args.PrevLogTerm) {
		nr.Logger.Printf("Nodo %d inconsistencia de log con líder %d: PrevLogIndex=%d PrevLogTerm=%d (local len=%d)",
			nr.Yo, args.IdLeader, args.PrevLogIndex, args.PrevLogTerm, len(nr.logEntries))
		return nil
	}

	// 6️ Eliminar entradas en conflicto y añadir nuevas
	nr.logEntries = nr.logEntries[:args.PrevLogIndex+1]
	nr.logEntries = append(nr.logEntries, args.Entries...)

	// 7️ Actualizar commitIndex
	if args.LeaderCommit > nr.commitIndex {
		nr.commitIndex = min(args.LeaderCommit, len(nr.logEntries)-1)
		go nr.aplicarEntradas()
	}

	results.Success = true
	results.Term = nr.currentTerm
	return nil
}

// ----- Metodos/Funciones a utilizar como clientes
//
//

// Ejemplo de código enviarPeticionVoto
//
// nodo int -- indice del servidor destino en nr.nodos[]
//
// args *RequestVoteArgs -- argumentos par la llamada RPC
//
// reply *RequestVoteReply -- respuesta RPC
//
// Los tipos de argumentos y respuesta pasados a CallTimeout deben ser
// los mismos que los argumentos declarados en el metodo de tratamiento
// de la llamada (incluido si son punteros
//
// Si en la llamada RPC, la respuesta llega en un intervalo de tiempo,
// la funcion devuelve true, sino devuelve false
//
// la llamada RPC deberia tener un timout adecuado.
//
// Un resultado falso podria ser causado por una replica caida,
// un servidor vivo que no es alcanzable (por problemas de red ?),
// una petición perdida, o una respuesta perdida
//
// Para problemas con funcionamiento de RPC, comprobar que la primera letra
// del nombre  todo los campos de la estructura (y sus subestructuras)
// pasadas como parametros en las llamadas RPC es una mayuscula,
// Y que la estructura de recuperacion de resultado sea un puntero a estructura
// y no la estructura misma.
func (nr *NodoRaft) enviarPeticionVoto(nodo int, args *ArgsPeticionVoto,
	reply *RespuestaPeticionVoto) bool {

	peer := nr.Nodos[nodo]

	// Llamamos remotamente al método del otro nodo
	err := peer.CallTimeout("NodoRaft.PedirVoto", args, reply, rpcTimeout*time.Millisecond)

	if err != nil {
		// Error de conexión o timeout -> el nodo remoto no ha respondido
		nr.Logger.Printf("Nodo %d: fallo al pedir voto a %d (%v)\n", nr.Yo, nodo, err)
		return false
	}

	// Si no hubo error, el reply ya está relleno
	nr.Logger.Printf("Nodo %d: respuesta de voto de %d: Term=%d, VoteGranted=%v\n",
		nr.Yo, nodo, reply.Term, reply.VoteGranted)

	return true
}

func (nr *NodoRaft) tratarRespuestaVoto(reply RespuestaPeticionVoto) {
	nr.Mux.Lock()
	defer nr.Mux.Unlock()

	// si quien responde está en mandato mayor=> soy "Follower"
	if reply.Term > nr.currentTerm {
		nr.currentTerm = reply.Term
		nr.role = Follower
		nr.votedFor = IntNOINICIALIZADO
		nr.IdLider = IntNOINICIALIZADO
		nr.resetTimer()
		nr.Logger.Printf("Nodo: %d vuelvo a Follower. Hay un nuevo mandato: %d\n",
			nr.Yo, nr.currentTerm)
		return
	}
	// si entre tanto he dejado de ser candidato
	if nr.role != Candidate {
		return
	}
	// recuento de votos
	if reply.VoteGranted && reply.Term == nr.currentTerm {
		nr.votesReceived++
		nr.Logger.Printf("Nodo: %d Recibido voto #%d en mandato %d\n",
			nr.Yo, nr.votesReceived, nr.currentTerm)
		// Si alcanzo mayoría me delcaro líder
		if nr.votesReceived > len(nr.Nodos)/2 {
			nr.role = Leader
			nr.IdLider = nr.Yo
			nr.votesReceived = 0
			nr.Logger.Printf("Nodo: %d Es elegido LIDER en mandato %d\n",
				nr.Yo, nr.currentTerm)
			// al ser Líder no vuelvo a solicitar elección
			if nr.timer != nil {
				nr.timer.Stop()
			}
			// Inicializar índices de replicación
			lastIdx := nr.lastLogIndex()
			for i := range nr.Nodos {
				if i == nr.Yo {
					continue
				}
				nr.nextIndex[i] = lastIdx + 1
				nr.matchIndex[i] = 0
			}
			// AQUI se debe comenzar a enviar latidos a los otros nodos
			go nr.enviarLatido()
			// nr.Logger.Printf("Nodo: %d en mandato %d ha iniciado envío de latidos\n",
			// 	nr.Yo, nr.currentTerm)
		}
	}
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

func (nr *NodoRaft) isCandidateUpToDate(cLastIdx, cLastTerm int) bool {
	myIdx := nr.lastLogIndex()
	myTerm := nr.lastLogTerm()
	if cLastTerm != myTerm {
		return cLastTerm > myTerm
	}
	return cLastIdx >= myIdx
}

func (nr *NodoRaft) enviarLatido() {
	repeater := time.NewTicker(heartbeatRate * time.Millisecond)
	defer repeater.Stop()
	for {
		nr.Mux.Lock()
		if nr.role != Leader {
			nr.Mux.Unlock()
			return
		}
		term := nr.currentTerm
		commit := nr.commitIndex
		yo := nr.Yo
		nodos := append([]rpctimeout.HostPort(nil), nr.Nodos...)
		// copiamos snapshot mínimo para soltar el lock ya
		nr.Mux.Unlock()

		for node, peer := range nodos {
			if node == yo {
				continue
			}
			go func(node int, peer rpctimeout.HostPort, term, commit int) {
				nr.Mux.Lock()
				// Prev por *ese* follower
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
				args := ArgAppendEntries{
					Term:         term,
					IdLeader:     yo,
					PrevLogIndex: prevIdx,
					PrevLogTerm:  prevTerm,
					Entries:      nil,
					LeaderCommit: commit,
				}
				nr.Mux.Unlock()
				nr.Logger.Printf("Nodo %d envía HB a %d con prevIdx= %d", yo, node, prevIdx)
				reply := Results{}
				err := peer.CallTimeout("NodoRaft.AppendEntries", &args, &reply, rpcTimeout*time.Millisecond)
				if err != nil {
					return
				}
				if reply.Term > term {
					nr.Mux.Lock()
					if reply.Term > nr.currentTerm {
						nr.currentTerm = reply.Term
						nr.role = Follower
						nr.votedFor = IntNOINICIALIZADO
						nr.IdLider = IntNOINICIALIZADO
						nr.resetTimer()
					}
					nr.Mux.Unlock()
				}
			}(node, peer, term, commit)
		}
		<-repeater.C
	}
}
