package fileManagerServer

import (
	"os"
	fileMangertypes "practica2/cmd/fileManager/types"
	"testing"
)

func createTempFile(content string) string {
	tmpfile, _ := os.CreateTemp("", "testfile*.txt")
	tmpfile.WriteString(content)
	tmpfile.Close()
	return tmpfile.Name()
}

func createTempEndpointsFile() string {
	tmpfile, _ := os.CreateTemp("", "endpoints*.txt")
	tmpfile.WriteString("localhost:9001\nlocalhost:9002\n")
	tmpfile.Close()
	return tmpfile.Name()
}

func createTempPeersFile() string {
	tmpfile, _ := os.CreateTemp("", "peers*.txt")
	tmpfile.WriteString("localhost:9003\nlocalhost:9004\n")
	tmpfile.Close()
	return tmpfile.Name()
}

func TestUpdateFile(t *testing.T) {
	endpoints := createTempEndpointsFile()
	peers := createTempPeersFile()
	file := createTempFile("initial")

	fs := New(1, endpoints, file, peers, false)

	args := &fileMangertypes.UpdateArgs{
		Content: "updated",
		Pos:     0,
		From:    0,
	}
	reply := &fileMangertypes.ReplyType{}

	err := fs.UpdateFile(args, reply)
	if err != nil || reply.Err != 0 {
		t.Errorf("UpdateFile failed: %v", err)
	}

	data, _ := os.ReadFile(file)
	if string(data) != "updated" {
		t.Errorf("Expected 'updated', got '%s'", string(data))
	}

	fs.Close()
}

func TestWriteFileAsReader(t *testing.T) {
	endpoints := createTempEndpointsFile()
	peers := createTempPeersFile()
	file := createTempFile("initial")

	fs := New(1, endpoints, file, peers, true) // reader node

	args := &fileMangertypes.WriteArgs{
		Content: "should fail",
		Pos:     0,
		From:    0,
	}
	reply := &fileMangertypes.ReplyType{}

	fs.WriteFile(args, reply)
	if reply.Err == 0 {
		t.Errorf("WriteFile should fail for reader node")
	}

	fs.Close()
}
