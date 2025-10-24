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
	endpoints := []string{"192.168.3.13:29280", "192.168.3.13:29281"}
	peers := []string{"192.168.3.13:29282", "192.168.3.13:29283"}

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
	endpoints := []string{"192.168.3.13:29284", "192.168.3.13:29285"}
	peers := []string{"192.168.3.13:29286", "192.168.3.13:29287"}

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

func TestMultipleTest(t *testing.T) {
	// 						reader				writer					reader				writer
	endpoints := []string{"192.168.3.13:29280", "192.168.3.14:29280", "192.168.3.15:29280", "192.168.3.16:29280"}
	testFile1 := "test_data_node1.txt"

	time.Sleep(1000 * time.Millisecond) // Esperar a que ambos servidores estén listos
	log.Println("Desde el test voy a pedir escribir en nodo: ", endpoints[1], " el texto: Distributed Write 1")
	go CallWrite(endpoints[1], "Distributed Write 1\n", 0, io.SeekEnd)

	log.Println("Desde el test voy a pedir escribir en nodo: ", endpoints[3], " el texto: Distributed Write 2")
	go CallWrite(endpoints[3], "Distributed Write 2\n", 0, io.SeekEnd)

	log.Println("Desde el test voy a pedir leer del nodo: ", endpoints[0])
	data, err := CallRead(endpoints[0], 0, 0)
	if err == nil {
		log.Println("Leido {\n", data, "\n}")
	} else {
		log.Println("Error en ", endpoints[0], " leyendo de fichero")
	}

	log.Println("Desde el test voy a pedir escribir en nodo: ", endpoints[1], " el texto: Distributed Write 3")
	go CallWrite(endpoints[3], "Distributed Write 3\n", 0, io.SeekEnd)

	log.Println("Desde el test voy a pedir escribir en nodo: ", endpoints[3], " el texto: Distributed Write 4")
	go CallWrite(endpoints[3], "Distributed Write 4\n", 0, io.SeekEnd)

	log.Println("Desde el test voy a pedir leer del nodo: ", endpoints[2])
	data, err = CallRead(endpoints[2], 0, 0)
	if err == nil {
		log.Println("Leido {\n", data, "\n}")
	} else {
		log.Println("Error en ", endpoints[2], " leyendo de fichero")
	}
	time.Sleep(1000 * time.Millisecond) // Esperar a que ambos servidores estén listos
	log.Println("Después de un descanso vuelvo a leer")
	log.Println("Desde el test voy a pedir leer del nodo: ", endpoints[0])
	data, err = CallRead(endpoints[0], 0, 0)
	if err == nil {
		log.Println("Leido {\n", data, "\n}")
	} else {
		log.Println("Error en ", endpoints[0], " leyendo de fichero")
	}
	log.Println("Contenido real del fichero:")

	text, _ := os.ReadFile(testFile1)
	log.Println(string(text))
	// if string(data) != "Distributed Write" {
	// 	t.Errorf("Se esperaba 'Distributed Write', se recibió '%s'", string(data))
	// }

	// Limpieza
	if string(text) != "Distributed Write" {
		t.Errorf("Se esperaba 'Distributed Write', se recibió '%s'", string(data))
	}
	// os.Remove(testFile1)

}
