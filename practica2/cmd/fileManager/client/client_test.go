package fileManagerClient

import (
	"io"
	"log"
	"os"
	fileManagerServer "practica2/cmd/fileManager/server"
	"testing"
	"time"
)

func createFile(path, content string) {
	os.WriteFile(path, []byte(content), 0644)
}

func createConfigFile(path string, lines []string) {
	f, _ := os.Create(path)
	for _, line := range lines {
		f.WriteString(line + "\n")
	}
	f.Close()
}

func TestCallReadSuccess(t *testing.T) {
	endpoints := []string{"localhost:9100", "localhost:9101"}
	peers := []string{"localhost:9202", "localhost:9203"}

	endpointsFile := "test_endpoints.txt"
	peersFile := "test_peers.txt"
	testFile1 := "test_data_node1.txt"
	testFile2 := "test_data_node2.txt"

	createConfigFile(endpointsFile, endpoints)
	createConfigFile(peersFile, peers)
	createFile(testFile1, "Hello World")
	createFile(testFile2, "Hello World")

	// Inicializar dos nodos
	fs1 := fileManagerServer.New(1, endpointsFile, testFile1, peersFile, true)
	fs2 := fileManagerServer.New(2, endpointsFile, testFile2, peersFile, false)

	go fs1.Listen()
	go fs2.Listen()
	defer fs1.Close()
	defer fs2.Close()

	time.Sleep(300 * time.Millisecond) // Esperar a que ambos servidores estén listos

	read, err := CallRead(endpoints[0], 11, 0)
	if err != nil {
		t.Fatalf("Error en CallRead: %v", err)
	}
	if read != "Hello World" {
		t.Errorf("Se esperaba 'Hello World', se recibió '%s'", read)
	}

	// Limpieza
	os.Remove(endpointsFile)
	os.Remove(peersFile)
	os.Remove(testFile1)
	os.Remove(testFile2)
}

func TestCallWriteSuccess(t *testing.T) {
	endpoints := []string{"localhost:9104", "localhost:9105"}
	peers := []string{"localhost:9206", "localhost:9207"}

	endpointsFile := "test_endpoints2.txt"
	peersFile := "test_peers2.txt"
	testFile1 := "test_data_node1.txt"
	testFile2 := "test_data_node2.txt"

	createConfigFile(endpointsFile, endpoints)
	createConfigFile(peersFile, peers)
	createFile(testFile1, "")
	createFile(testFile2, "")

	// Inicializar dos nodos: nodo 1 escritor, nodo 2 lector (solo para que responda al RA)
	fs1 := fileManagerServer.New(1, endpointsFile, testFile1, peersFile, true)  // lector
	fs2 := fileManagerServer.New(2, endpointsFile, testFile2, peersFile, false) // escritor

	go fs1.Listen()
	go fs2.Listen()
	defer fs1.Close()
	defer fs2.Close()

	log.Println("1 ", fs1.LocalAdress())
	log.Println("2 ", fs2.LocalAdress())

	time.Sleep(1000 * time.Millisecond) // Esperar a que ambos servidores estén listos
	log.Println("Desde el test voy a pedir escribir en nodo: ", endpoints[1])
	err := CallWrite(endpoints[1], "Distributed Write", 0, io.SeekEnd)

	if err != nil {
		t.Fatalf("Error en CallWrite: %v", err)
	}

	data, _ := os.ReadFile(testFile1)
	if string(data) != "Distributed Write" {
		t.Errorf("Se esperaba 'Distributed Write', se recibió '%s'", string(data))
	}

	// Limpieza
	os.Remove(endpointsFile)
	os.Remove(peersFile)
	os.Remove(testFile1)
	os.Remove(testFile2)
}
