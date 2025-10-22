package fileManager

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/rpc"
	"os"
	"practica2/ra"
)

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

func (fm *FileServer) UpdateFile(args *UpdateArgs, reply *ReplyType) error {
	log.Println("Soy ", fm.me, " he recibido Update file call")
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

func (fm *FileServer) WriteFile(args *WriteArgs, reply *ReplyType) error {
	log.Println("Soy ", fm.me, " he recibido Write file call")
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
			fc := FileClient{}
			err = fc.CallUpdate(fm.endpoints[i+1], args.Content, args.Pos, args.From)
			if err != nil {
				log.Printf("El endpoint %s no pudo escribir el fichero\n", ep)
			}
		}
	}
	reply.Err = 0
	reply.Data = nil
	return nil
}

func (fm *FileServer) ReadFile(args *ReadArgs, reply *ReplyType) error {
	log.Println("Soy ", fm.me, " he recibido Read file call")
	if !fm.distributedMutex.Reader {
		reply.Err = -1
		reply.Data = []byte("It is not a reader node")
		return nil
	}
	log.Println("Soy ", fm.me, " trylock")
	fm.distributedMutex.PreProtocol()
	defer fm.distributedMutex.PostProtocol()
	log.Println("Soy ", fm.me, " estoy en sc")
	data, err := os.ReadFile(fm.filename)

	if err != nil {
		reply.Err = -1
		reply.Data = []byte("Error reading file")
	} else {
		reply.Data = data
		reply.Err = 0
	}
	log.Println("Soy ", fm.me, " salgo de sc")
	return nil
}

func (fm *FileServer) ServerOn() {
	l, err := net.Listen("tcp", fm.endpoints[fm.me-1])
	checkError(err)
	defer l.Close()

	for {
		conn, err := l.Accept()
		log.Println("NUevo cliente RPC conectado ", conn.RemoteAddr())
		if err != nil {
			continue
		}
		go rpc.ServeConn(conn)
	}
}

func NewServer(me int, endpointsFile string, filename string, peerFile string, reader bool) *FileServer {

	fm := &FileServer{
		me:               me,
		endpoints:        ParseEndpoints(endpointsFile),
		filename:         filename,
		distributedMutex: ra.New(me, peerFile, reader),
	}

	rpc.Register(fm)

	return fm
}

func (fm *FileServer) Close() {
	fm.distributedMutex.Stop()
}
