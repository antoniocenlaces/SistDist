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
	"bufio"
	"encoding/gob"
	"log"
	"net"
	"os"
	"practica1/com"
	"sync"
)

var mtxNGOS sync.Mutex                  //Mutex global para el acceso a nGos
var nGOs int = 0                        // nGos representa en numero de gorutines activas en el sistema
var finSync chan bool = make(chan bool) //Canal para sincronizar el main con la finilazcion del resto de gorotines

// PRE: true
// POST: devuelve un vector de string con los endpoints del fichero filename y si no hay algun error devuelve error nil
// y si hay algun error devuelve el vector de endpoints vacio y el error correspondiente
func readEndpoints(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var endpoints []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			endpoints = append(endpoints, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return endpoints, nil
}

// PRE: true
// POST: devuelve un vector de string con los endpoints y si no hay algun error devuelve error nil
// y si hay algun error devuelve el vector de endpoints vacio y el error correspondiente
func getEndpoints() ([]string, error) {
	endpointsFile := os.Args[2]
	var endpoints []string
	var err error

	if endpoints, err = readEndpoints(endpointsFile); err != nil {
		log.Println("Error reading endpoints:", err)
	}
	return endpoints, err
}

// PRE: conn.open
// POST: Devuelve el estado de haber enviado msg
func sendMsg(conn net.Conn, msg interface{}) error {
	encoder := gob.NewEncoder(conn)
	return encoder.Encode(msg)
}

// PRE: conn.open
// POST: Devuelve el estado de haber leido msg y rellena msg
func readMsg(conn net.Conn, msg interface{}) error {
	decoder := gob.NewDecoder(conn)
	return decoder.Decode(msg)
}

// PRE: endpoint de el worker correspondiente al procedimiento
// POST: Coordina la conexion con el worker envia las peticiones y recibe las respuesta agregandolas
// al canal de respuesta. Cuando se finaliza el trabajo manda la request de fin al worker y cierra la conexion.
// En definitva es la representacion del worker en el servidor
func processRequest(endpoint string, requestChan chan com.Request, replyChan chan com.Reply, quit chan bool) {
	incrementarGos()
	defer decrementarGos()

	for {
		select {
		//Nueva request a procesar, si no hay mensajes en el canal se queda bloqueado hasta que haya uno para leer
		// por el worker asociado
		case request := <-requestChan:
			conn, err := net.Dial("tcp", endpoint)
			com.CheckError(err)
			//Envio request al worker
			var reply com.Reply
			err = sendMsg(conn, request)
			com.CheckError(err)
			log.Println("Request enviado al worker ", conn.RemoteAddr())

			//Leo la respuesta del worker y la añado al canal
			err = readMsg(conn, &reply)
			com.CheckError(err)
			log.Println("Reply recibida del worker ", conn.RemoteAddr())
			replyChan <- reply
		case <-quit: //El trabajo finaliza hay que cerrar el worker
			conn, err := net.Dial("tcp", endpoint)
			com.CheckError(err)
			fin := com.Request{Id: -1, Interval: com.TPInterval{Min: 0, Max: 0}}
			err = sendMsg(conn, fin) //Envio request del fin al worker
			com.CheckError(err)
			log.Println("Request de fin enviado al worker ", conn.RemoteAddr())
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
	que los worker lo procesen.

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
	err := readMsg(conn, &request) //Leo la peticion del cliente
	com.CheckError(err)
	if request.Id == -1 {
		log.Println("Request de fin de programa")
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
func replyHandle(requestConn *map[int]net.Conn, replyChan chan com.Reply, quit chan bool, mu *sync.Mutex) {
	incrementarGos()
	defer decrementarGos()
	for {
		select {
		case reply := <-replyChan: //Va sacando replys del canal si no hay ninguna se bloquea

			mu.Lock()
			conn, ok := (*requestConn)[reply.Id] //Leo del mapa en exclusion mutua la ID
			mu.Unlock()

			if ok {
				err := sendMsg(conn, reply) //Envio la reply al cliente
				com.CheckError(err)
				mu.Lock()
				delete(*requestConn, reply.Id) //Saco del mapa la request
				mu.Unlock()
				conn.Close()
				log.Println("Reply to", conn.RemoteAddr())
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
	args := os.Args

	readChan := make(chan com.Request) //canal de lectura de peticiones para la pool
	replychan := make(chan com.Reply)  // canal de respuestas para la pool
	replyMap := make(map[int]net.Conn) //mapa de relaciones clave : ID de peticion valor: Conexion abierta para la peticion ID
	quitChan := make(chan bool)        // Canal para indicar el fin del trabajo
	var mu sync.Mutex                  //Mutex para acceso concurrente al mapa replyMap

	if len(args) != 3 {
		log.Println("Error: endpoint missing: go run server.go ip:port endpoint-file")
		os.Exit(1)
	}
	endPoints, err := getEndpoints() //obtengo los endpoints para los workers
	com.CheckError(err)

	endpoint := args[1] //Endpoint de escucha
	listener, err := net.Listen("tcp", endpoint)
	com.CheckError(err)

	defer listener.Close()
	log.SetFlags(log.Lshortfile | log.Lmicroseconds)

	log.Println("***** Listening for new connection in endpoint ", endpoint)

	//Genero los handle de los workers
	for i := range endPoints {
		log.Println("Gorutine para worker ", endPoints[i])
		go processRequest(endPoints[i], readChan, replychan, quitChan)
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
