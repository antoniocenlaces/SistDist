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
	"log"
	"net"
	"practica2/ms"
	"sync"
	"time"
)

type Request struct {
	Clock  int
	Pid    int
	Reader bool
}

type Reply struct{}

type File struct {
	Text string
}

type RASharedDB struct {
	Me         int
	totalNodes int
	OurSeqNum  int
	HigSeqNum  int
	OutRepCnt  int
	ReqCS      bool
	RepDefd    []int // Replies Deferred for this node. The Pid of delayed REQUESTS
	// ms has the message box for this node, my peers, channel for communication of Message,
	// channel for finalization and my own Id
	ms         *ms.MessageSystem
	done       chan bool
	chrep      chan bool
	Mutex      sync.Mutex // mutex para proteger concurrencia sobre las variables
	AllReplied *sync.Cond
	Reader     bool
}

func New(me int, usersFile string, reader bool) *RASharedDB {
	messageTypes := []ms.Message{Request{}, Reply{}, File{}} // Message is interface{}, here is defined
	// the different types of messages for the MessageSystem
	msgs := ms.New(me, usersFile, messageTypes)
	var ra RASharedDB
	ra = RASharedDB{
		Me:         me,
		totalNodes: msgs.TotalNodes(),
		OurSeqNum:  0,
		HigSeqNum:  0,
		OutRepCnt:  0,
		ReqCS:      false,
		RepDefd:    []int{},
		ms:         &msgs,
		done:       make(chan bool),
		chrep:      make(chan bool),
		Mutex:      sync.Mutex{},
		AllReplied: sync.NewCond(&ra.Mutex),
		Reader:     reader,
	}
	return &ra
}

// Pre: Verdad
// Post: Realiza  el  PreProtocol  para el  algoritmo de
//
//	Ricart-Agrawala Generalizado
func (ra *RASharedDB) PreProtocol() {
	// Using ra mutex common variables are updated to REQUEST critical section
	ra.Mutex.Lock()
	ra.ReqCS = true
	ra.OurSeqNum = ra.HigSeqNum + 1
	ra.OutRepCnt = ra.totalNodes - 1
	ra.Mutex.Unlock()
	var msg ms.Message
	msg = Request{Clock: ra.OurSeqNum, Pid: ra.OurSeqNum, Reader: ra.Reader}
	// a message with my Id, my function (reader or writer) and my actual sequence number are sent to all other nodes
	for i, ep := range endpoints {
		if i+1 != ra.Me {
			go func(ep string, m Message) {
				for {
					conn, err := net.Dial("tcp", ep)
					if err != nil {
						log.Println("Error connecting to ", ep, ":", err)
						time.Sleep(1 * time.Second)
						continue
					}
					if err = sendMsg(conn, m); err != nil {
						log.Println("Error messaging to ", ep, ":", err)
						time.Sleep(1 * time.Second)
						continue
					}
					conn.Close()
					break
				}
			}(ep, msg)
		}
	}
	// Now we wait for all other nodes to answer my REQUEST
	ra.Mutex.Lock()
	for ra.OutstandingReplies > 0 {
		ra.AllReplied.Wait() // I go to sleep while all other sent their notification
		// a Broadcast will wake up me and I'll recover ra.Mutex
	}
	ra.Mutex.Unlock()

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
