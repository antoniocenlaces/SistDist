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
	me         int
	totalNodes int
	ourSeqNum  int
	higSeqNum  int
	outRepCnt  int
	reqCS      bool
	repDefd    []int // Replies Deferred for this node. The Pid of delayed REQUESTS
	// ms has the message box for this node, my peers, channel for communication of Message,
	// channel for finalization and my own Id
	ms         *ms.MessageSystem
	done       chan bool
	mutex      sync.Mutex // mutex para proteger concurrencia sobre las variables
	allReplied *sync.Cond // conditional variable for PreProtocol() wait all peers to send REPLY
	Reader     bool       // true: this peer is a reader; flase: this peer is a writer
}

func New(whoAmI int, usersFile string, reader bool) *RASharedDB {
	messageTypes := []ms.Message{Request{}, Reply{}} // Message is interface{}, here is defined
	// the different types of messages for the MessageSystem registered for use with gob
	msgs := ms.New(whoAmI, usersFile, messageTypes)

	ra := RASharedDB{
		me:         whoAmI,
		totalNodes: msgs.TotalNodes(),
		ourSeqNum:  0,
		higSeqNum:  0,
		outRepCnt:  0,
		reqCS:      false,
		repDefd:    []int{},
		ms:         &msgs,
		done:       make(chan bool),
		mutex:      sync.Mutex{},
		Reader:     reader,
	}
	ra.allReplied = sync.NewCond(&ra.mutex)
	log.Println("Creada estrucutra ra para nodo nº: ", ra.me, " en: ", msgs.Peers[ra.me-1], " como Reader: ", ra.Reader)
	go handleReceivedMessages(&ra)
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
	log.Println("ra de nodo nº: ", ra.me, " pide entrar en SC como Reader: ", ra.Reader)
	// Using ra mutex common variables are updated to REQUEST critical section
	ra.mutex.Lock()
	ra.reqCS = true
	ra.ourSeqNum = ra.higSeqNum + 1
	ra.outRepCnt = ra.totalNodes - 1
	log.Println("ra de nodo nº: ", ra.me, " tiene el mutex y ourSeqNum=", ra.ourSeqNum, " outRepCnt=", ra.outRepCnt)
	ra.mutex.Unlock()
	log.Println("ra de nodo nº: ", ra.me, " ha soltado el mutex")
	msg := Request{Clock: ra.ourSeqNum, Pid: ra.me, Reader: ra.Reader}
	// a message with my Id, my function (reader or writer) and my actual sequence number are sent to all other nodes
	for i := 1; i <= ra.totalNodes; i++ {
		if i != ra.me {
			log.Println("ra de nodo nº: ", ra.me, " envía Request a ", i)
			go ra.ms.Send(i, msg)
		}
	}
	// Now we wait for all other nodes to answer my REQUEST
	ra.mutex.Lock()
	for ra.outRepCnt > 0 {
		log.Println("ra de nodo nº: ", ra.me, " tiene el mutex y se duerme en .Wait()")
		ra.allReplied.Wait() // I go to sleep while all other sent their notification
		// a Broadcast will wake up me and I'll recover ra.mutex
	}
	ra.mutex.Unlock()
	log.Println("ra de nodo nº: ", ra.me, " ha sido despertado y devuelve el mutex")
}

// Pre: Verdad
// Post: Realiza  el  PostProtocol  para el  algoritmo de
//
//	Ricart-Agrawala Generalizado
func (ra *RASharedDB) PostProtocol() {
	var wg sync.WaitGroup // used to count number of goroutines launched and wait for them to end properly
	ra.mutex.Lock()
	myRepDef := ra.repDefd
	ra.repDefd = []int{}
	ra.reqCS = false
	ra.mutex.Unlock()
	msg := Reply{}

	for _, pid := range myRepDef {
		// notification to node j
		// ra.mutex.Lock()
		// ra.Reply_deferred[j] = false
		// ra.mutex.Unlock()
		wg.Add(1) // add one goroutine
		go func(pid int, m Reply) {
			defer wg.Done()
			ra.ms.Send(pid, m)
		}(pid, msg)

	}

	wg.Wait() // wait all goroutines to finish

}

func (ra *RASharedDB) Stop() {
	ra.ms.Stop()
	ra.done <- true
}

func processReply(n *RASharedDB) {
	n.mutex.Lock()
	n.outRepCnt--
	log.Println("ra de nodo nº: ", n.me, " tiene el mutex y ha recibido un Reply. outRepCnt=", n.outRepCnt)
	if n.outRepCnt <= 0 {
		n.outRepCnt = 0
		log.Println("ra de nodo nº: ", n.me, " ha recibido Reply de todos y podrá entrar en SC")
		n.allReplied.Broadcast() // wake up the waiter
		log.Println("después de despertar al PreProtocol he procesado todos los Reply")
	}
	n.mutex.Unlock()

}

func processRequest(n *RASharedDB, msg Request) {
	n.mutex.Lock()
	n.higSeqNum = max(n.higSeqNum, msg.Clock)
	deferIt := n.reqCS && !(n.Reader && msg.Reader) &&
		((msg.Clock > n.ourSeqNum) || (msg.Clock == n.ourSeqNum && msg.Pid > n.me))
	if deferIt {
		n.repDefd = append(n.repDefd, msg.Pid)
	}
	n.mutex.Unlock()
	if !deferIt {
		n.ms.Send(msg.Pid, Reply{}) // then I REPLY to the REQUEST
	}
}

// Called when a REPLY message arrives
// HandleReceivedMessages accepts connections and dispatches them.
// It listens until quit is closed; when quit is closed we close the listener
// which makes Accept return an error and the loop stops.
func handleReceivedMessages(n *RASharedDB) {
	for {
		select {
		case <-n.done:
			return
		default:
			msg, ok := n.ms.Receive()
			if ok {
				switch m := msg.(type) {
				case Request:
					processRequest(n, m)
				case Reply:
					processReply(n)
				default:
					log.Println("Error in hanling message received: unknown type. Node: ", n.me)
				}
			}
		}
	}
}
