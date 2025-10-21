/*
* AUTOR: Rafael Tolosana Calasanz
* ASIGNATURA: 30221 Sistemas Distribuidos del Grado en Ingeniería Informática
*			Escuela de Ingeniería y Arquitectura - Universidad de Zaragoza
* FECHA: septiembre de 2021
* FICHERO: ricart-agrawala.go
* DESCRIPCIÓN: Implementación del algoritmo de Ricart-Agrawala Generalizado en Go
 */
package ra

import (
	"practica2/ms"
	"sync"
)

type Request struct {
	Clock  int
	Pid    int
	Reader bool
}

type Reply struct{}

type RASharedDB struct {
	OurSeqNum int
	HigSeqNum int
	OutRepCnt int
	ReqCS     bool
	RepDefd   []int // Replies Deferred for this node. The Pid of delayed REQUESTS
	// ms has the message box for this node, my peers, channel for communication of Message,
	// channel for finalization and my own Id
	ms         *ms.MessageSystem
	done       chan bool
	chrep      chan bool
	Mutex      sync.Mutex // mutex para proteger concurrencia sobre las variables
	AllReplied *sync.Cond
}

func New(me int, usersFile string) *RASharedDB {
	messageTypes := []ms.Message{Request{}, Reply{}} // Message is interface{}, here is defined
	// the different types of messages for the MessageSystem
	msgs := ms.New(me, usersFile, messageTypes)
	var ra RASharedDB
	ra = RASharedDB{0, 0, 0, false, []int{}, &msgs, make(chan bool), make(chan bool), sync.Mutex{}, sync.NewCond(&ra.Mutex)}
	// TODO completar
	return &ra
}

// Pre: Verdad
// Post: Realiza  el  PreProtocol  para el  algoritmo de
//
//	Ricart-Agrawala Generalizado
func (ra *RASharedDB) PreProtocol() {
	// TODO completar
}

// Pre: Verdad
// Post: Realiza  el  PostProtocol  para el  algoritmo de
//
//	Ricart-Agrawala Generalizado
func (ra *RASharedDB) PostProtocol() {
	// TODO completar
}

func (ra *RASharedDB) Stop() {
	ra.ms.Stop()
	ra.done <- true
}
