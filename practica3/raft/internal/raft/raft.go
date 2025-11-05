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
	kLogToStdout = false

	// Cambiar esto para salida de logs en un directorio diferente
	kLogOutputDir = "./logs_raft/"
	// base y techo del timer para nueva elección (ms)
	baseTimer = 150
	ceilTimer = 300
	// Timeout de la llamada RPC en ms (ajustar al entorno donde se ejecuta)
	rpcTimeout = 200
	// tiempo entre latidos enviados por el líder (ms)
	heartbeatRate = 50
	// definición de estados del servidor
	Follower state = iota
	Candidate
	Leader
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

// Tipo de dato Go que representa un solo nodo (réplica) de raft
type NodoRaft struct {
	Mux sync.Mutex // Mutex para proteger acceso a estado compartido

	// Host:Port de todos los nodos (réplicas) Raft, en mismo orden
	Nodos   []rpctimeout.HostPort
	Yo      int         // indice de este nodos en campo array "nodos"
	IdLider int         // índice en Nodos del líder
	Logger  *log.Logger // para facilitar depuración

	//

	// Estado persistente en todos los nodos
	currentTerm int                // último mandato visto por el nodo. 0 al inicializar.
	votedFor    int                // candidato que recibió mi voto en mandato actual
	logEntries  []*AplicaOperacion // registro de entradas en máquina estados

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
	role          state         // "Follower", "Candidate", "Leader"
	votesReceived int           // votos durante elección
	electionReset time.Duration // última vez que recibió mensaje válido
	timer         *time.Timer   // para gestionar timeout de elecciones
	// Canal para aplicar operaciones comprometidas a la máquina de estados
	canalAplicarOperacion chan AplicaOperacion
}

// resetTimer reinicia el temporizador de elección del nodo.
// Cancela el temporizador anterior (si existe) y programa uno nuevo
// con una duración aleatoria en el rango [150ms, 300ms].
func (nr *NodoRaft) resetTimer() {
	// Duración aleatoria para evitar colisiones simultáneas de elección
	dur := time.Duration(baseTimer+rand.Intn(ceilTimer-baseTimer)) * time.Millisecond
	// para evitar condiciones de carrera solo lanzamos AfterFunc() si no hay timer
	if nr.timer == nil {
		nr.timer = time.AfterFunc(dur, nr.iniciarEleccion)
	} else {
		nr.timer.Reset(dur)
	}
	nr.Logger.Printf("[Nodo %d] Temporizador reiniciado (timeout: %v)", nr.Yo, nr.electionReset)
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

	nr.currentTerm = 0
	nr.votedFor = IntNOINICIALIZADO
	nr.logEntries = make([]*AplicaOperacion, 0)

	nr.commitIndex = 0
	nr.lastApplied = 0

	nr.nextIndex = make(map[int]int, len(nr.Nodos))
	nr.matchIndex = make(map[int]int, len(nr.Nodos))

	nr.role = Follower
	nr.votesReceived = 0

	nr.resetTimer()

	return nr
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
	var yo int = nr.Yo
	var mandato int = nr.currentTerm
	var esLider bool = nr.Yo == nr.IdLider
	var idLider int = nr.IdLider

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

	// Vuestro codigo aqui

	return indice, mandato, EsLider, idLider, valorADevolver
}

func (nr *NodoRaft) iniciarEleccion() {
	nr.Mux.Lock()
	// incrementa mandato para esta nueva elección
	nr.currentTerm++
	nr.votedFor = nr.Yo
	nr.votesReceived = 1 // voto por mí mismo
	nr.role = Candidate
	nr.resetTimer() // para evitar que vuelva a dispararse elección
	nr.Logger.Printf("Nodo %d inicia elección para mandato %d\n", nr.Yo, nr.currentTerm)

	args := ArgsPeticionVoto{
		Term:        nr.currentTerm,
		CandidateId: nr.Yo,
	}
	for node, peer := range nr.Nodos {
		if node == nr.Yo {
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
	nr.Mux.Unlock()
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
	// si aún no he votado o bien voté por este candidato le repito el voto
	if nr.votedFor == IntNOINICIALIZADO || nr.votedFor == peticion.CandidateId {
		nr.votedFor = peticion.CandidateId
		reply.VoteGranted = true
		// reinicia el temporizador de elección
		nr.resetTimer()
		nr.Logger.Printf("Nodo: %d Vota por %d en mandato %d", nr.Yo,
			nr.votedFor, nr.currentTerm)
	}
	reply.Term = nr.currentTerm
	return nil
}

type ArgAppendEntries struct {
	Term     int // mandato actual del líder
	IdLeader int // id del líder para redirigir clientes
	// después se introducirán los campos para control de replicas
}

type Results struct {
	Term    int  // mandato actual del follower
	Success bool // true si el follower acepta la entrada (o latido)
}

// Metodo de tratamiento de llamadas RPC AppendEntries
func (nr *NodoRaft) AppendEntries(args *ArgAppendEntries,
	results *Results) error {
	nr.Mux.Lock()
	defer nr.Mux.Unlock()

	results.Success = false
	results.Term = nr.currentTerm
	// si quien envía entrada está en mandato inferior se ignora
	if args.Term < nr.currentTerm {
		nr.Logger.Printf("Nodo %d rechaza AppendEntries de %d (mandato %d < %d)",
			nr.Yo, args.IdLeader, args.Term, nr.currentTerm)
		return nil
	}
	// si llega de mandato igual o superior al mío, me actualizo y acepto la entrada
	// por ahora solo llegarán latidos
	if args.Term >= nr.currentTerm {
		nr.currentTerm = args.Term
		nr.role = Follower
		nr.IdLider = args.IdLeader
		results.Success = true
		nr.resetTimer()
		nr.Logger.Printf("Nodo %d acepta latido de líder %d (mandato %d)",
			nr.Yo, args.IdLeader, nr.currentTerm)
	}
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
		}
		// AQUI se debe comenzar a enviar latidos a los otros nodos
		go nr.enviarLatido()
	}
}

func (nr *NodoRaft) enviarLatido() {
	// repeater usa time.NewTicker para crear evento repetitivo
	repeater := time.NewTicker(heartbeatRate * time.Millisecond)
	defer repeater.Stop()
	// mientras sea líder envía latidos a todos los nodos
	for {
		nr.Mux.Lock()
		// si deja de ser líder para los latidos
		if nr.role != Leader {
			nr.Mux.Unlock()
			return
		}
		// crea args para enivar el latido dentro de bucle
		// para reflejar si cambia el mandato en esa ejecución del bucle
		args := ArgAppendEntries{
			Term:     nr.currentTerm,
			IdLeader: nr.Yo,
		}
		// bucle para enviar a todos los nodos
		for node, peer := range nr.Nodos {
			if node != nr.Yo {
				go func(peer rpctimeout.HostPort, node int, args ArgAppendEntries) {
					reply := Results{}
					err := peer.CallTimeout("NodoRaft.AppendEntries", &args, &reply,
						rpcTimeout*time.Millisecond)
					if err != nil && reply.Term > nr.currentTerm {
						// hay un nodo con mandato superior => Follower
						nr.Mux.Lock()
						nr.currentTerm = reply.Term
						nr.role = Follower
						nr.votedFor = IntNOINICIALIZADO
						nr.IdLider = IntNOINICIALIZADO
						nr.resetTimer()
						nr.Mux.Unlock()
					}
				}(peer, node, args)
			}
		}
		nr.Mux.Lock()
		// espera a que transcurra heartbeatRate ms y llegará señal
		<-repeater.C
	}

}
