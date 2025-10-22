package fileManagerServer

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/rpc"
	"os"
	fileManagerClient "practica2/cmd/fileManager/client"
	fileMangertypes "practica2/cmd/fileManager/types"
	"practica2/ra"
)

type FileServer struct {
	me               int
	endpoints        []string
	filename         string
	distributedMutex *ra.RASharedDB
}

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

func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		os.Exit(1)
	}
}
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
		// io.ErrShortWrite es el error estándar para escrituras incompletas
		return io.ErrShortWrite
	}

	return nil

}

func (fm *FileServer) UpdateFile(args *fileMangertypes.UpdateArgs, reply *fileMangertypes.ReplyType) error {
	//log.Println("Soy ", fm.me, " he recibido Update file call")
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

func (fm *FileServer) WriteFile(args *fileMangertypes.WriteArgs, reply *fileMangertypes.ReplyType) error {
	//log.Println("Soy ", fm.me, " he recibido Write file call")
	if fm.distributedMutex.Reader {
		reply.Err = -1
		reply.Data = []byte("It is not a writer node")
		return nil
	}
	fm.distributedMutex.PreProtocol()
	defer fm.distributedMutex.PostProtocol()
	err := writeFile(args.Content, args.Pos, args.From, fm.filename)
	if err != nil {
		reply.Err = -1
		reply.Data = []byte("Error writing on file")
		return nil
	}

	for i, ep := range fm.endpoints {
		if i != fm.me-1 {

			err = fileManagerClient.CallUpdate(ep, args.Content, args.Pos, args.From)
			if err != nil {
				log.Printf("El endpoint %s no pudo escribir el fichero\n", ep)
			}
		}
	}
	reply.Err = 0
	reply.Data = nil
	return nil
}

func (fm *FileServer) LocalAdress() string {
	return fm.endpoints[fm.me-1]
}

func (fm *FileServer) ReadFile(args *fileMangertypes.ReadArgs, reply *fileMangertypes.ReplyType) error {
	//log.Println("Soy ", fm.me, " he recibido Read file call")
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

func (fm *FileServer) Listen() {
	l, err := net.Listen("tcp", fm.endpoints[fm.me-1])
	checkError(err)
	defer l.Close()

	for {
		conn, err := l.Accept()
		//log.Println("NUevo cliente RPC conectado ", conn.RemoteAddr())
		if err != nil {
			continue
		}
		go rpc.ServeConn(conn)
	}
}

func New(me int, endpointsFile string, filename string, peerFile string, reader bool) *FileServer {

	fm := &FileServer{
		me:               me,
		endpoints:        parseEndpoints(endpointsFile),
		filename:         filename,
		distributedMutex: ra.New(me, peerFile, reader),
	}

	rpc.Register(fm)

	return fm
}

func (fm *FileServer) Close() {
	fm.distributedMutex.Stop()
}
