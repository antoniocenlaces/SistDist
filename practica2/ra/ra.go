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
	messageTypes := []ms.Message{Request{}, Reply{}} // Message is interface{}, here is defined
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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
	msg := Request{Clock: ra.OurSeqNum, Pid: ra.OurSeqNum, Reader: ra.Reader}
	// a message with my Id, my function (reader or writer) and my actual sequence number are sent to all other nodes
	for i := 1; i <= ra.totalNodes; i++ {
		if i != ra.Me {
			go ra.ms.Send(i, msg)
		}
	}
	// Now we wait for all other nodes to answer my REQUEST
	ra.Mutex.Lock()
	for ra.OutRepCnt > 0 {
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
	var wg sync.WaitGroup // used to count number of goroutines launched and wait for them to end properly
	ra.Mutex.Lock()
	ra.ReqCS = false
	ra.Mutex.Unlock()
	msg := Reply{}
	for _, pid := range ra.RepDefd {
		// notification to node j
		// ra.Mutex.Lock()
		// ra.Reply_deferred[j] = false
		// ra.Mutex.Unlock()
		wg.Add(1) // add one goroutine
		go func(pid int, m Reply) {
			defer wg.Done()
			ra.ms.Send(pid, m)
		}(pid, msg)

	}
	ra.Mutex.Lock()
	ra.RepDefd = []int{}
	ra.Mutex.Unlock()
	wg.Wait() // wait all goroutines to finish
}

func (ra *RASharedDB) Stop() {
	ra.ms.Stop()
	ra.done <- true
}

func processReply(n *RASharedDB) {
	n.Mutex.Lock()
	n.OutRepCnt--
	if n.OutRepCnt <= 0 {
		n.OutRepCnt = 0
		n.AllReplied.Broadcast() // wake up the waiter
	}
	n.Mutex.Unlock()
}

func processRequest(n *RASharedDB, msg Request) {
	n.Mutex.Lock()
	n.HigSeqNum = max(n.HigSeqNum, msg.Clock)
	deferIt := n.ReqCS && !(n.Reader && msg.Reader) &&
		((msg.Clock > n.OurSeqNum) || (msg.Clock == n.OurSeqNum && msg.Pid > n.Me))
	n.Mutex.Unlock()
	if deferIt {
		n.Mutex.Lock()
		n.RepDefd = append(n.RepDefd, msg.Pid)
		n.Mutex.Unlock()
	} else {
		n.ms.Send(msg.Pid, Reply{}) // then I REPLY to the REQUEST
	}
}

// Called when a REPLY message arrives
// HandleReceivedMessages accepts connections and dispatches them.
// It listens until quit is closed; when quit is closed we close the listener
// which makes Accept return an error and the loop stops.
func HandleReceivedMessages(n *RASharedDB) {
	select {
	case <-n.done:
		return
	default:
		msg := n.ms.Receive()
		switch m := msg.(type) {
		case Request:
			processRequest(n, m)
		case Reply:
			processReply(n)
		default:
			log.Println("Error in hanling message received: unknown type. Node: ", n.Me)
		}
	}
}
