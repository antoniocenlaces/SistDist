package fileManager

import (
	"errors"
	"log"
	"net/rpc"
	"practica2/fileManagerTypes"
)

func (fm *FileClient) CallRead(endpoint string, len int, pos int) (read string, err error) {
	client, err := rpc.Dial("tcp", endpoint)
	if err != nil {
		return "", err
	}

	defer client.Close()

	readArgs := ReadArgs
		Len: len,
		Pos: pos,
	}
	readReply := ReplyType{}
	log.Println("LLamo a FileManager.ReadFile para el nodo ", endpoint)
	err = client.Call("FileServer.ReadFile", &readArgs, &readReply)
	log.Println("He recibido respuesta ")
	if err != nil {
		return "", err
	}
	if readReply.Err != 0 {
		return "", errors.New(string(readReply.Data))
	}

	return string(readReply.Data), nil
}

func (fm *FileClient) CallWrite(endpoint string, content string, pos int, from int) (err error) {
	client, err := rpc.Dial("tcp", endpoint)
	if err != nil {
		return err
	}
	defer client.Close()
	writeArgs := WriteArgs{
		Content: content,
		Pos:     pos,
		From:    from,
	}
	writeReply := ReplyType{}
	log.Println("LLamo a FileManager.WriteFile para el nodo ", endpoint)
	err = client.Call("FileServer.WriteFile", &writeArgs, &writeReply)
	log.Println("He recibido respuesta ")
	if err != nil {
		return err
	}
	if writeReply.Err != 0 {
		return errors.New(string(writeReply.Data))
	}

	return nil
}

func (fm *FileClient) CallUpdate(endpoint string, content string, pos int, from int) (err error) {
	client, err := rpc.Dial("tcp", endpoint)
	if err != nil {
		return err
	}
	defer client.Close()
	updateArgs := UpdateArgs{
		Content: content,
		Pos:     pos,
		From:    from,
	}
	updateReply := ReplyType{}
	log.Println("LLamo a FileManager.UpdateFile para el nodo ", endpoint)
	err = client.Call("FileServer.UpdateFile", &updateArgs, &updateReply)
	log.Println("He recibido respuesta ")
	if err != nil {
		return err
	}
	if updateReply.Err != 0 {
		return errors.New(string(updateReply.Data))
	}

	return nil
}
