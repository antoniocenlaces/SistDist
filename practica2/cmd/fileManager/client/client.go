package fileManagerClient

import (
	"errors"
	"net/rpc"
	fileMangertypes "practica2/cmd/fileManager/types"
)

func CallRead(endpoint string, len int, pos int) (read string, err error) {
	client, err := rpc.Dial("tcp", endpoint)
	if err != nil {
		return "", err
	}

	defer client.Close()

	readArgs := fileMangertypes.ReadArgs{
		Len: len,
		Pos: pos,
	}
	readReply := fileMangertypes.ReplyType{}
	//log.Println("LLamo a FileManager.ReadFile para el nodo ", endpoint)
	err = client.Call("FileServer.ReadFile", &readArgs, &readReply)
	//log.Println("He recibido respuesta ")
	if err != nil {
		return "", err
	}
	if readReply.Err != 0 {
		return "", errors.New(string(readReply.Data))
	}

	return string(readReply.Data), nil
}

func CallWrite(endpoint string, content string, pos int, from int) (err error) {
	client, err := rpc.Dial("tcp", endpoint)
	if err != nil {
		return err
	}
	defer client.Close()
	writeArgs := fileMangertypes.WriteArgs{
		Content: content,
		Pos:     pos,
		From:    from,
	}
	writeReply := fileMangertypes.ReplyType{}
	//log.Println("LLamo a FileManager.WriteFile para el nodo ", endpoint)
	err = client.Call("FileServer.WriteFile", &writeArgs, &writeReply)
	//log.Println("He recibido respuesta ")
	if err != nil {
		return err
	}
	if writeReply.Err != 0 {
		return errors.New(string(writeReply.Data))
	}

	return nil
}

func CallUpdate(endpoint string, content string, pos int, from int) (err error) {
	client, err := rpc.Dial("tcp", endpoint)
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
	//log.Println("LLamo a FileManager.UpdateFile para el nodo ", endpoint)
	err = client.Call("FileServer.UpdateFile", &updateArgs, &updateReply)
	//log.Println("He recibido respuesta ")
	if err != nil {
		return err
	}
	if updateReply.Err != 0 {
		return errors.New(string(updateReply.Data))
	}

	return nil
}
