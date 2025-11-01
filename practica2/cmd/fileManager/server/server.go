package fileManagerServer

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/rpc"
	"os"
	fileMangertypes "practica2/cmd/fileManager/types"
	"practica2/ra"
)

// FileServer representa un nodo del sistema de ficheros distribuido.
// Cada nodo mantiene su propio fichero local y coordina operaciones
// de lectura y escritura con el resto mediante exclusión mutua distribuida,
// implementada mediante el algoritmo de Ricart-Agravala
type FileServer struct {
	me               int            // Identificador del nodo actual (1..N)
	endpoints        []string       // Direcciones de todos los nodos del sistema
	filename         string         // Nombre del fichero local que gestiona este nodo
	distributedMutex *ra.RASharedDB // Mecanismo de exclusión mutua distribuida (lectores/escritores)
	listener         net.Listener
}

// parseEndpoints
//
// Descripción:
//
//	Lee un fichero de texto donde cada línea contiene la dirección de un nodo (endpoint).
//	Construye y devuelve un slice con todos los endpoints del sistema.
//
// Pre:
//   - endpointsFile != "" → Ruta válida al fichero de configuración.
//
// Post:
//   - Devuelve un slice []string con las direcciones IP:puerto leídas del fichero.
//   - Si ocurre un error al abrir o leer el fichero, el programa finaliza mediante checkError.
func parseEndpoints(endpointsFile string) (endpoints []string) {
	file, err := os.Open(endpointsFile)
	checkError(err)
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		endpoints = append(endpoints, scanner.Text())
	}
	return endpoints
}

// checkError
//
// Descripción:
//
//	Verifica si ha ocurrido un error. Si existe, muestra un mensaje fatal y detiene la ejecución.
//
// Pre:
//   - Puede recibir err = nil o un valor de error válido.
//
// Post:
//   - Si err != nil → imprime el error en stderr y finaliza la ejecución del proceso.
//   - Si err == nil → no realiza ninguna acción.
func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		os.Exit(1)
	}
}

// writeFile
//
// Descripción:
//
//	Escribe el texto indicado en un fichero local, posicionándose según los parámetros indicados.
//
// Pre:
//   - pos >= 0 → Posición válida en el fichero.
//   - fileName != "" → Nombre del fichero local donde escribir.
//   - whence ∈ {0, 1, 2} → Modo de desplazamiento en el fichero (io.SeekStart, io.SeekCurrent, io.SeekEnd). La posición será calculada a partir  del desplazamiento
//
// Post:
//   - Si la escritura se completa correctamente, devuelve err = nil.
//   - Si ocurre un error (pos inválida, escritura incompleta o fallo de E/S), devuelve el error correspondiente.
func writeFile(text string, pos int, whence int, fileName string) error {
	if pos < 0 {
		return fmt.Errorf("invalid pos: %d", pos)
	}
	file, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Seek(int64(pos), whence)
	if err != nil {
		return err
	}

	n, err := file.Write([]byte(text))
	if err != nil {
		return err
	}
	if n < len(text) {
		return io.ErrShortWrite
	}

	return nil
}

// UpdateFile
//
// Descripción:
//
//	Maneja la llamada RPC “FileServer.UpdateFile”.
//	Actualiza **solo el fichero local** del nodo (no propaga la actualización).
//
// Pre:
//   - args.Content != "" → Contenido a escribir en el fichero local.
//   - args.Pos >= 0 → Posición válida en el fichero.
//   - args.From ∈ {0, 1, 2} → Indica el desplazamiento en el fichero (io.SeekStart, io.SeekCurrent, io.SeekEnd). La posición será calculada a partir  del desplazamiento
//   - fm.filename debe existir o ser accesible para escritura.
//
// Post:
//   - Si la operación se realiza correctamente, reply.Err = 0 y reply.Data = nil.
//   - Si ocurre un error, reply.Err = -1 y reply.Data contiene el mensaje de error.
func (fm *FileServer) UpdateFile(args *fileMangertypes.UpdateArgs, reply *fileMangertypes.ReplyType) error {
	log.Println("Soy nodo nº: ", fm.me, " en: ", fm.listener.Addr().String(), " como Reader: ", fm.distributedMutex.Reader, "actualizo mi fichero con: ", args.Content)
	err := writeFile(args.Content, args.Pos, args.From, fm.filename)
	if err != nil {
		reply.Err = -1
		reply.Data = []byte(err.Error())
		return nil
	}
	reply.Err = 0
	reply.Data = nil
	return nil
}

// WriteFile
//
// Descripción:
//
//	Maneja la llamada RPC “FileServer.WriteFile”.
//	Ejecuta una **escritura distribuida**, aplicando el cambio en este nodo
//	y propagándolo al resto mediante llamadas “UpdateFile”.
//	La operación está protegida por el protocolo de exclusión mutua distribuida (RA).
//
// Pre:
//   - fm.distributedMutex.Reader == false → Solo nodos escritores pueden ejecutar esta operación.
//   - args.Content != "" → Contenido válido para escribir.
//   - args.Pos >= 0 → Posición válida en el fichero.
//   - args.From ∈ {0, 1, 2} →  Indica el desplazamiento en el fichero (io.SeekStart, io.SeekCurrent, io.SeekEnd). La posición será calculada a partir  del desplazamiento
//
// Post:
//   - Si la escritura y propagación son exitosas, reply.Err = 0.
//   - Si el nodo no es escritor o ocurre un error de escritura, reply.Err = -1 y reply.Data contiene el mensaje.
func (fm *FileServer) WriteFile(args *fileMangertypes.WriteArgs, reply *fileMangertypes.ReplyType) error {
	log.Println("Me piden escribir en nodo nº: ", fm.me, " en: ", fm.listener.Addr().String(), " como Reader: ", fm.distributedMutex.Reader)
	if fm.distributedMutex.Reader {
		reply.Err = -1
		reply.Data = []byte("It is not a writer node")
		return nil
	}

	fm.distributedMutex.PreProtocol()
	log.Println("ENTRADA EN CS: Nodo nº: ", fm.me)
	defer fm.distributedMutex.PostProtocol()

	err := writeFile(args.Content, args.Pos, args.From, fm.filename)
	if err != nil {
		reply.Err = -1
		reply.Data = []byte("Error writing on file")
		return nil
	}
	log.Println("Nodo nº: ", fm.me, "ha escrito en su fichero y comunica cambios al resto")
	// Propagar la actualización a los demás nodos
	for i, ep := range fm.endpoints {
		if i != fm.me-1 {
			log.Println("Nodo nº: ", fm.me, "comunica cambios al nodo: ", i+1)
			err = fm.callUpdate(i+1, args.Content, args.Pos, args.From)
			if err != nil {
				log.Printf("El endpoint %s no pudo escribir el fichero\n", ep)
			}
		}
	}

	reply.Err = 0
	reply.Data = nil
	log.Println("SALIDA DE CS: Nodo nº: ", fm.me)
	return nil
}

// LocalAdress
//
// Descripción:
//
//	Devuelve la dirección TCP asociada al nodo actual.
//
// Pre:
//   - fm.me ∈ [1, len(fm.endpoints)].
//
// Post:
//   - Retorna la dirección TCP/IP:puerto correspondiente a este nodo.
func (fm *FileServer) LocalAdress() string {
	return fm.endpoints[fm.me-1]
}

// ReadFile
//
// Descripción:
//
//	Maneja la llamada RPC “FileServer.ReadFile”.
//	Permite a los nodos lectores acceder al contenido del fichero local.
//
// Pre:
//   - fm.distributedMutex.Reader == true → Solo nodos lectores pueden realizar esta operación.
//   - fm.filename debe existir o ser accesible para lectura.
//
// Post:
//   - Si la lectura es exitosa, reply.Err = 0 y reply.Data contiene el contenido del fichero.
//   - Si el nodo no es lector o ocurre un error de lectura, reply.Err = -1 y reply.Data contiene el error.
//
// De momento está función no tiene implementado el leer desde una posición una longitus especifica
// Ahora mismo se lee el fichero entero y se deuvlve el contenido al completo
func (fm *FileServer) ReadFile(args *fileMangertypes.ReadArgs, reply *fileMangertypes.ReplyType) error {
	log.Println("Piden leer en nodo nº: ", fm.me, " estoy en: ", fm.listener.Addr().String(), " como Reader: ", fm.distributedMutex.Reader)
	if !fm.distributedMutex.Reader {
		reply.Err = -1
		reply.Data = []byte("It is not a reader node")
		return nil
	}

	fm.distributedMutex.PreProtocol()
	defer fm.distributedMutex.PostProtocol()

	data, err := os.ReadFile(fm.filename)
	if err != nil {
		reply.Err = -1
		reply.Data = []byte("Error reading file")
	} else {
		reply.Data = data
		reply.Err = 0
	}
	return nil
}

// Listen
//
// Descripción:
//
//	Inicia un servidor RPC que escucha conexiones entrantes y atiende solicitudes de otros nodos o clientes.
//
// Pre:
//   - fm.endpoints[fm.me-1] debe ser una dirección TCP válida y disponible.
//
// Post:
//   - El servidor queda en bucle aceptando conexiones RPC y sirviendo peticiones concurrentemente.
func (fm *FileServer) Listen() {
	var err error
	fm.listener, err = net.Listen("tcp", fm.endpoints[fm.me-1])
	checkError(err)
	defer fm.listener.Close()

	rpcServer := rpc.NewServer()
	rpcServer.Register(fm)

	for {
		conn, err := fm.listener.Accept()
		if err != nil {
			continue
		}
		go rpcServer.ServeConn(conn)
	}
}

// callUpdate
//
// Descripción:
//
//	Realiza una llamada RPC a otro nodo para ejecutar “FileServer.UpdateFile”.
//	Actualiza el fichero local remoto sin propagar más allá de ese nodo.
//
// Pre:
//   - pid ∈ [1, len(fm.endpoints)] → Identificador válido del nodo destino.
//   - content != "" → Contenido que se desea escribir.
//   - pos >= 0 → Posición de escritura.
//   - from ∈ {0, 1, 2} →  Indica el desplazamiento en el fichero (io.SeekStart, io.SeekCurrent, io.SeekEnd). La posición será calculada a partir  del desplazamiento
//
// Post:
//   - Si la actualización remota es exitosa, devuelve err = nil.
//   - Si ocurre un error de conexión o ejecución, devuelve el error correspondiente.
func (fm *FileServer) callUpdate(pid int, content string, pos int, from int) (err error) {
	client, err := rpc.Dial("tcp", fm.endpoints[pid-1])
	if err != nil {
		return err
	}
	defer client.Close()

	updateArgs := fileMangertypes.UpdateArgs{
		Content: content,
		Pos:     pos,
		From:    from,
	}
	updateReply := fileMangertypes.ReplyType{}

	err = client.Call("FileServer.UpdateFile", &updateArgs, &updateReply)
	if err != nil {
		return err
	}
	if updateReply.Err != 0 {
		return errors.New(string(updateReply.Data))
	}

	return nil
}

// New
//
// Descripción:
//
//	Crea e inicializa una nueva instancia de FileServer con los parámetros indicados.
//	Registra el servicio RPC y prepara el nodo para escuchar peticiones.
//
// Pre:
//   - me > 0 && me <= número de endpoints.
//   - endpointsFile, filename, peerFile → rutas válidas.
//   - reader ∈ {true, false} → Indica si el nodo será lector o escritor.
//
// Post:
//   - Devuelve un puntero a FileServer inicializado y registrado en el sistema RPC.
func New(me int, endpointsFile string, filename string, peerFile string, reader bool) *FileServer {
	fm := &FileServer{
		me:               me,
		endpoints:        parseEndpoints(endpointsFile),
		filename:         filename,
		distributedMutex: ra.New(me, peerFile, reader),
	}

	// rpc.Register(fm)
	log.Println("Lanzado FileServer como nodo nº: ", fm.me, " en: ", fm.endpoints[me-1], " como Reader: ", fm.distributedMutex.Reader)
	return fm
}

// Close
//
// Descripción:
//
//	Finaliza el servidor cerrando el mecanismo de exclusión mutua distribuida.
//
// Pre:
//   - fm.distributedMutex debe estar inicializado.
//
// Post:
//   - Detiene el proceso de sincronización distribuida y libera recursos asociados.
func (fm *FileServer) Close() {
	fm.distributedMutex.Stop()
}
