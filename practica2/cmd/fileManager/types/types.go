package fileManagerTypes

// WriteArgs
//
// Descripción:
//
//	Estructura de argumentos utilizada en las llamadas RPC “FileServer.WriteFile”.
//	Representa una operación de escritura en el **fichero distribuido**.
//
// Campos:
//   - Content (string): Contenido que se desea escribir.
//   - Pos (int): Posición dentro del fichero donde se iniciará la escritura.
//   - From (int): Indica el desplazamiento en el fichero a partir del cual será calculada la posción.
//     0 = io.SeekStart,
//     1 = io.SeekCurrent,
//     2 = io.SeekEnd.
type WriteArgs struct {
	Content string
	Pos     int
	From    int
}

// ReadArgs
//
// Descripción:
//
//	Estructura de argumentos utilizada en las llamadas RPC “FileServer.ReadFile”.
//	Define los parámetros necesarios para una lectura en el fichero remoto.
//
// Campos:
//   - Len (int): Número de bytes que se desean leer.
//   - Pos (int): Posición dentro del fichero desde la cual iniciar la lectura.
type ReadArgs struct {
	Len int
	Pos int
}

// UpdateArgs
//
// Descripción:
//
//	Estructura de argumentos utilizada en las llamadas RPC “FileServer.UpdateFile”.
//	Representa una operación de actualización **local** en el fichero de un nodo,
//	normalmente enviada desde el nodo primario tras una escritura distribuida.
//
// Campos:
//   - Content (string): Contenido que se escribirá en el fichero local.
//   - Pos (int): Posición en el fichero donde aplicar la actualización.
//   - From (int): Indica el desplazamiento en el fichero a partir del cual será calculada la posción.
//     0 = io.SeekStart,
//     1 = io.SeekCurrent,
//     2 = io.SeekEnd.
type UpdateArgs struct {
	Content string
	Pos     int
	From    int
}

// ReplyType
//
// Descripción:
//
//	Estructura de respuesta estándar utilizada por todos los procedimientos RPC
//	del sistema de gestión de ficheros distribuidos.
//
// Campos:
//   - Err (int): Código de error de la operación:
//     ; 0 = Éxito,
//     ; -1 = Error o fallo en la operación.
//   - Data ([]byte): Datos devueltos por la operación (contenido leído o mensaje de error).
type ReplyType struct {
	Err  int
	Data []byte
}
