package fileManager

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"os"
	"practica2/ra"
)

type WriteArgs struct {
	Content string
	Pos     int
	From    int
}

type ReadArgs struct {
	Len int
	Pos int
}

type UpdateArgs struct {
	Content string
	Pos     int
	From    int
}

type ReplyType struct {
	Err  int
	Data []byte
}

type FileManager struct {
	me               int
	endpoints        []string
	filename         string
	distributedMutex *ra.RASharedDB
}

func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		os.Exit(1)
	}
}

func writeFile(text string, pos int, from int, fileName string) error {
	file, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Seek(int64(pos), from)
	if err != nil {

		return err
	}

	n, err := file.Write([]byte(text))
	if err != nil || n < len(text) {

		return err
	}

	return nil

}

func (fm *FileManager) UpdateFile(args *UpdateArgs, reply *ReplyType) {
	err := writeFile(args.Content, args.Pos, args.From, fm.filename)
	if err != nil {
		reply.Err = -1
		reply.Data = []byte(err.Error())
	}
}

func (fm *FileManager) WriteFile(args *WriteArgs, reply *ReplyType) {
	if fm.distributedMutex.Reader {
		reply.Err = -1
		reply.Data = []byte("It is not a writer node")
		return
	}
	fm.distributedMutex.PreProtocol()
	defer fm.distributedMutex.PostProtocol()
	err := writeFile(args.Content, args.Pos, args.From, fm.filename)
	if err != nil {
		reply.Err = -1
		reply.Data = []byte("Error writing on file")
		return
	}

	for i, ep := range fm.endpoints {
		if ep != fm.endpoints[fm.me-1] {
			err = fm.CallUpdate(i+1, args.Content, args.Pos, args.From)
			if err != nil {
				log.Printf("El endpoint %s no pudo escribir el fichero\n", ep)
			}
		}
	}

}

func (fm *FileManager) ReadFile(args *ReadArgs, reply *ReplyType) {
	if !fm.distributedMutex.Reader {
		reply.Err = -1
		reply.Data = []byte("It is not a reader node")
		return
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

func (fm *FileManager) CallRead(pid int, len int, pos int) (read string, err error) {
	client, err := rpc.Dial("tcp", fm.endpoints[pid-1])
	if err != nil {
		return "", err
	}

	readArgs := ReadArgs{
		Len: len,
		Pos: pos,
	}
	readReply := ReplyType{}

	err = client.Call("FileManager.ReadFile", readArgs, readReply)
	if err != nil {
		return "", err
	}
	if readReply.Err != 0 {
		return "", errors.New(string(readReply.Data))
	}

	return string(readReply.Data), nil
}

func (fm *FileManager) CallWrite(pid int, content string, pos int, from int) (err error) {
	client, err := rpc.Dial("tcp", fm.endpoints[pid-1])
	if err != nil {
		return err
	}

	writeArgs := WriteArgs{
		Content: content,
		Pos:     pos,
		From:    from,
	}
	writeReply := ReplyType{}

	err = client.Call("FileManager.WriteFile", writeArgs, writeReply)
	if err != nil {
		return err
	}
	if writeReply.Err != 0 {
		return errors.New(string(writeReply.Data))
	}

	return nil
}

func (fm *FileManager) CallUpdate(pid int, content string, pos int, from int) (err error) {
	client, err := rpc.Dial("tcp", fm.endpoints[pid-1])
	if err != nil {
		return err
	}

	updateArgs := UpdateArgs{
		Content: content,
		Pos:     pos,
		From:    from,
	}
	updateReply := ReplyType{}

	err = client.Call("FileManager.UpdateFile", updateArgs, updateReply)
	if err != nil {
		return err
	}
	if updateReply.Err != 0 {
		return errors.New(string(updateReply.Data))
	}

	return nil
}
func (fm *FileManager) ServerOn() {
	go func() {
		l, err := net.Listen("tcp", fm.endpoints[fm.me-1])
		checkError(err)
		defer l.Close()

		for {
			conn, err := l.Accept()
			if err != nil {
				continue
			}
			go rpc.ServeConn(conn)
		}
	}()
}

func New(me int, endpointsFile string, filename string, peerFile string, reader bool) *FileManager {

	fm := &FileManager{
		me:               me,
		endpoints:        parseEndpoints(endpointsFile),
		filename:         filename,
		distributedMutex: ra.New(me, peerFile, reader),
	}

	rpc.Register(fm)

	return fm
}

func (fm *FileManager) Close() {
	fm.distributedMutex.Stop()
}
