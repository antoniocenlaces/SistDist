package fileManager

import (
	"bufio"
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

type FileServer struct {
	me               int
	endpoints        []string
	filename         string
	distributedMutex *ra.RASharedDB
}
type FileClient struct {
}

func ParseEndpoints(endpointsFile string) (endpoints []string) {

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
