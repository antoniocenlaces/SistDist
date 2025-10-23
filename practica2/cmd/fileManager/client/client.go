package fileManagerClient

import (
	"errors"
	"net/rpc"
	fileMangertypes "practica2/cmd/fileManager/types"
)

// CallRead
//
// Pre:
//   - endpoint != "" → Debe especificarse la dirección del servidor remoto (por ejemplo, "localhost:8080").
//   - len > 0 → La longitud de lectura solicitada debe ser mayor que cero.
//   - pos >= 0 → La posición inicial en el fichero desde donde se desea leer no puede ser negativa.
//
// Post:
//   - Si la operación tiene éxito, devuelve el contenido leído del fichero remoto (en forma de cadena) y err = nil.
//   - Si ocurre un error en la conexión RPC o en la lectura del fichero remoto, devuelve una cadena vacía y el error correspondiente.
func CallRead(endpoint string, length int, pos int) (read string, err error) {
	client, err := rpc.Dial("tcp", endpoint) // Establece la conexión RPC con el servidor
	if err != nil {
		return "", err
	}
	defer client.Close()

	// Estructuras de datos que deben coincidir con las definidas en el servidor
	readArgs := fileMangertypes.ReadArgs{
		Len: length,
		Pos: pos,
	}
	readReply := fileMangertypes.ReplyType{}

	// Llamada al procedimiento remoto
	err = client.Call("FileServer.ReadFile", &readArgs, &readReply)

	// Error en la comunicación RPC
	if err != nil {
		return "", err
	}

	// Error en la lectura del fichero remoto
	if readReply.Err != 0 {
		return "", errors.New(string(readReply.Data))
	}

	return string(readReply.Data), nil
}

// CallWrite
// Descripción:
//
//	Realiza una escritura en el **fichero distribuido remoto**, propagando el cambio a todos los nodos
//
// Pre:
//   - endpoint != "" → Dirección válida del servidor remoto.
//   - content != "" → Debe existir contenido para escribir.
//   - pos >= 0 → La posición de escritura no puede ser negativa.
//   - from ∈ {0, 1, 2} → Indica desde donde se va aplicar el valor de pos (por ejemplo: 0=io.SeekStart, 1=io.SeekCurrent, 2=io.SeekEnd).
//
// Post:
//   - Si la escritura distribuida se completa correctamente, devuelve err = nil.
//   - Si ocurre un error en la conexión RPC o durante la escritura, devuelve el error correspondiente.
func CallWrite(endpoint string, content string, pos int, from int) (err error) {
	client, err := rpc.Dial("tcp", endpoint) // Establece la conexión RPC con el servidor
	if err != nil {
		return err
	}
	defer client.Close()
	// Estructuras de datos que deben coincidir con las definidas en el servidor
	writeArgs := fileMangertypes.WriteArgs{
		Content: content,
		Pos:     pos,
		From:    from,
	}
	writeReply := fileMangertypes.ReplyType{}
	// Llamada al procedimiento remoto
	err = client.Call("FileServer.WriteFile", &writeArgs, &writeReply)
	// Error en la comunicación RPC
	if err != nil {
		return err
	}
	// Error en la escritura del fichero remoto
	if writeReply.Err != 0 {
		return errors.New(string(writeReply.Data))
	}

	return nil
}
