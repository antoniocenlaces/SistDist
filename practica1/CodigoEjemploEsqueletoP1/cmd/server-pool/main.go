/*
* AUTOR: Rafael Tolosana Calasanz y Unai Arronategui
* ASIGNATURA: 30221/39521 - Sistemas Distribuidos
*	       Escuela de Ingeniería y Arquitectura - Universidad de Zaragoza
* FECHA: septiembre de 2022
* FICHERO: server-draft.go
* DESCRIPCIÓN: contiene la funcionalidad esencial para realizar los servidores
*				correspondientes a la práctica 1
 */
package main

import (
	"encoding/gob"
	"log"
	"net"
	"os"
	"practica1/com"
	"strconv"
	"sync"
)

var mtxNGOS sync.Mutex                  //Mutex global para el acceso a nGos
var nGOs int = 0                        // nGos representa en numero de gorutines activas en el sistema
var finSync chan bool = make(chan bool) //Canal para sincronizar el main con la finilazcion del resto de gorotines

// PRE: verdad = !foundDivisor
// POST: IsPrime devuelve verdad si n es primo y falso en caso contrario
func isPrime(n int) (foundDivisor bool) {
	foundDivisor = false
	for i := 2; (i < n) && !foundDivisor; i++ {
		foundDivisor = (n%i == 0)
	}
	return !foundDivisor
}

// PRE: interval.A < interval.B
// POST: FindPrimes devuelve todos los números primos comprendidos en el
//
//	intervalo [interval.A, interval.B]
func findPrimes(interval com.TPInterval) (primes []int) {
	for i := interval.Min; i <= interval.Max; i++ {
		if isPrime(i) {
			primes = append(primes, i)
		}
	}
	return primes
}

/*
PRE:

	true

POST:

	Mientras no se cierre el canal quit, lee request del canal y si no se queda bloqueado
	esperando nuevas request, las procesa y la intrudce en el canal de reply
*/
func processRequest(requestChan chan com.Request, replyChan chan com.Reply, quit chan bool) {
	incrementarGos()
	defer decrementarGos()

	for {
		select {
		//Nueva request a procesar, si no hay mensajes en el canal se queda bloqueado hasta que haya uno para leer
		// por el
		case request := <-requestChan:
			primes := findPrimes(request.Interval)
			reply := com.Reply{Id: request.Id, Primes: primes}
			replyChan <- reply // Mete la respuesta calculada para que se envia a su respectivo cliente
		case <-quit: //Lectura de canal de quit o cierre del canal, si esto sucede se termina la funcion
			return
		}
	}
}

/*
PRE:

	requestConn != nil && conn == open && listner != nil && mu != nil

POST:

	Gestiona la conexion conn con el Listener listner y genera una entrada en el mapa para
	relacionar cada ID de peticion (request.Id) con su conexion (conn), metiendo la petición al canal de lectura
	que la pool la procese.

	Si rquest.Id == -1 es el fin del trabajo por lo que se cierra el canal quit para notificar a todos que tienen
	que terminar, además cierra el listener asociado para que salga del accept en caso de que este bloqueado esperando
	una nueva petición.
*/
func handleConnection(conn net.Conn, requestConn *map[int]net.Conn,
	readChan chan com.Request, quit chan bool, listner *net.Listener, mu *sync.Mutex) {
	incrementarGos()
	defer decrementarGos()
	log.Println("New connection from ", conn.RemoteAddr())
	//Se obtiene la request de la conexion con el cliente conn
	var request com.Request
	decoder := gob.NewDecoder(conn)
	err := decoder.Decode(&request)
	com.CheckError(err) //Si hay algun error haciendo la deserializacion error y salgo del programa

	if request.Id == -1 {
		close(quit) //Cierre del canal, notifica fin del trabajo
		(*listner).Close()
	} else {
		// Seccion critica:  para evitar que haya escrituras concurrentes en el mapa y quede en un estado inconsistente
		mu.Lock()
		(*requestConn)[request.Id] = conn
		mu.Unlock()
		//Fin seccion crítica
		readChan <- request //Añado mensaje al canal
	}

}

/*
PRE:

	requestConn != nil

POST:

	Mientras que no se haya finalizado el trabajo, el se leen las respuestas generadas por la
	pool de goroutines y si el reply.Id tiene una conexion asociada, se contesta al cliente con
	la reply leida del canal. Después se quita del mapa esa petición, ya que ha sido procesada y
	se cierra la conexion.
*/
func replyHandle(requestConn *map[int]net.Conn, replyChan chan com.Reply, quit chan bool, mtx *sync.Mutex) {
	incrementarGos()
	defer decrementarGos()
	for {
		select {
		case reply := <-replyChan: //Va sacando replys del canal si no hay ninguna se bloquea
			mtx.Lock()
			conn, ok := (*requestConn)[reply.Id] //Leo del mapa en exclusion mutua la ID
			mtx.Unlock()
			if ok {
				encoder := gob.NewEncoder(conn)
				encoder.Encode(&reply) //Contesto al cliente

				mtx.Lock()
				delete(*requestConn, reply.Id) //Saco del mapa la request
				mtx.Unlock()

				conn.Close()
				log.Println("Reply to", conn.RemoteAddr())
			} else {
				log.Println("Not found connection for replyID ", reply.Id)
			}
		case <-quit:
			return
		}
	}
}

// PRE: true
// POST: aumenta en 1 el valor de go routines activas, en exclusion mutua
func incrementarGos() {
	defer mtxNGOS.Unlock()

	mtxNGOS.Lock()
	nGOs++

}

// PRE: true
// POST: Disminuye en 1 el valor de go routines activas, en exclusion mutua, si nGOS <= 0 entonces no hay que hacer nada.
// Si al disminuir el numero es 0 entonces indicamos que han terminado todas las gorutinas para que el main acabe.
func decrementarGos() {
	defer mtxNGOS.Unlock()
	mtxNGOS.Lock()

	if nGOs > 0 {
		nGOs--
		if nGOs == 0 {
			finSync <- true
		}
	}

}

func main() {
	readChan := make(chan com.Request) //canal de lectura de peticiones para la pool
	replychan := make(chan com.Reply)  // canal de respuestas para la pool
	replyMap := make(map[int]net.Conn) //mapa de relaciones clave : ID de peticion valor: Conexion abierta para la peticion ID
	quitChan := make(chan bool)        // Canal para indicar el fin del trabajo
	var mu sync.Mutex                  //Mutex para acceso concurrente al mapa replyMap

	args := os.Args
	if len(args) != 3 {
		log.Println("Error: endpoint missing: go run server.go ip:port nPool")
		os.Exit(1)
	}
	endpoint := args[1] //Endpoint de escucha
	listener, err := net.Listen("tcp", endpoint)

	com.CheckError(err)
	defer listener.Close()
	log.SetFlags(log.Lshortfile | log.Lmicroseconds)

	log.Println("***** Listening for new connection in endpoint ", endpoint)

	n, err := strconv.Atoi(args[2]) // Numero de gorutines en el pool
	com.CheckError(err)

	// Genera la pool de n goroutines
	for i := 0; i < n; i++ {
		go processRequest(readChan, replychan, quitChan)
	}
	//Lanzo el thread de respuesta a los clientes
	go replyHandle(&replyMap, replychan, quitChan, &mu)

	//Bucle de aceptacion de clientes
	for {
		conn, err := listener.Accept()
		select {
		case <-quitChan: //Si el canal recibe algo o se cierra hay que terminar
			log.Println("Deteniendo servidor....")
			<-finSync // Se queda bloqueado hasta que nGOs llega a 0, es decir ,han terminado todas las goroutines y se puede acabar el proceso
			log.Println("Exiting program")
			return
		default:
			com.CheckError(err)
			//Thread para la gestion de la conexion para no quedar bloqueado en el recv y
			// poder aceptar nuevos clientes concurrentes
			go handleConnection(conn, &replyMap, readChan, quitChan, &listener, &mu)
		}

	}

}
