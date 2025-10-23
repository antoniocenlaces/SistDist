/*
* AUTOR: Rafael Tolosana Calasanz
* ASIGNATURA: 30221 Sistemas Distribuidos del Grado en Ingeniería Informática
*			Escuela de Ingeniería y Arquitectura - Universidad de Zaragoza
* FECHA: septiembre de 2021
* FICHERO: ms_test.go
* DESCRIPCIÓN: Implementación de un sistema de mensajería asíncrono, insipirado en el Modelo Actor
 */
package ms

import (
	"os"
	"testing"
	"time"
)

type Request struct {
	Id int
}

type Reply struct {
	Response string
}

func createTempPeersFile(content string) string {
	tmpfile, _ := os.CreateTemp("", "peers*.txt")
	tmpfile.WriteString(content)
	tmpfile.Close()
	return tmpfile.Name()
}

func TestParsePeersValid(t *testing.T) {
	path := createTempPeersFile("localhost:8001\nlocalhost:8002\n")
	peers := parsePeers(path)
	if len(peers) != 2 {
		t.Errorf("Expected 2 peers, got %d", len(peers))
	}
}

func TestParsePeersInvalid(t *testing.T) {
	path := createTempPeersFile("localhost:8001\n")
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic due to insufficient peers")
		}
	}()
	parsePeers(path)
}

func TestSendReceiveMessage(t *testing.T) {
	path := createTempPeersFile("localhost:8001\nlocalhost:8002\n")
	p1 := New(1, path, []Message{Request{}, Reply{}})
	p2 := New(2, path, []Message{Request{}, Reply{}})

	time.Sleep(100 * time.Millisecond) // Give time for listeners to start

	p1.Send(2, Request{Id: 42})
	msg, _ := p2.Receive()
	request := msg.(Request)

	if request.Id != 42 {
		t.Errorf("Expected Request{42}, got Request{%d}", request.Id)
	}

	p2.Send(1, Reply{"OK"})
	msg, _ = p1.Receive()
	reply := msg.(Reply)

	if reply.Response != "OK" {
		t.Errorf("Expected Reply{OK}, got Reply{%s}", reply.Response)
	}

	p1.Stop()
	p2.Stop()
}
