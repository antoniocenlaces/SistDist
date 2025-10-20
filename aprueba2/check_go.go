package main

import (
	"encoding/gob"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

func CheckError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		os.Exit(1)
	}
}

// PRE: conn.open
// POST: Devuelve el estado de haber enviado msg
func sendMsg(conn net.Conn, msg interface{}) error {
	encoder := gob.NewEncoder(conn)
	return encoder.Encode(msg)
}

// PRE: conn.open
// POST: Devuelve el estado de haber leido msg y rellena msg
func readMsg(conn net.Conn, msg interface{}) error {
	decoder := gob.NewDecoder(conn)
	return decoder.Decode(msg)
}

type Message struct {
	Id     int
	Reader bool
	Seq    int
	Reply  bool // true is a Message replying my REQUEST
}

type Node struct {
	Mu                 sync.Mutex   // to control exclusive access to common variables
	Me                 int          // my own node number
	OutstandingReplies int          // number of REPLY still ot received
	AllReplied         *sync.Cond   // condition to chek if all have sent their REPLY
	MySeq              int          // my own squence number to be sent with my REQUEST
	HighestSeq         int          // highest sequence numbe I have seen in any REQUEST sent or received
	RequestedSC        bool         // to control when I have requested to enter CS
	Reader             bool         //true is a reader; false is a writer;
	Reply_deferred     map[int]bool // dictionary with node numbers that I have deferred their REQUEST
	TotalNodes         int
}

func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func NewNode(me int, function bool, totalNodes int) *Node {
	n := &Node{}
	n.Me = me
	n.TotalNodes = totalNodes
	n.AllReplied = sync.NewCond(&n.Mu)
	n.OutstandingReplies = 0
	n.MySeq = 0
	n.HighestSeq = 0
	n.RequestedSC = false
	n.Reader = function
	n.Reply_deferred = make(map[int]bool, totalNodes)
	return n
}

func (myNode *Node) RequestCS(endpoints []string) {
	// Using myNode mutex common variables are updated to REQUEST critical section
	myNode.Mu.Lock()
	myNode.RequestedSC = true
	myNode.MySeq = myNode.HighestSeq + 1
	myNode.OutstandingReplies = myNode.TotalNodes - 1
	myNode.Mu.Unlock()
	msg := Message{Id: myNode.Me, Reader: myNode.Reader, Seq: myNode.MySeq, Reply: false}
	// a message with my Id, my function (reader or writer) and my actual sequence number are sent to all other nodes
	for i, ep := range endpoints {
		if i+1 != myNode.Me {
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
	myNode.Mu.Lock()
	for myNode.OutstandingReplies > 0 {
		myNode.AllReplied.Wait() // I go to sleep while all other sent their notification
		// a Broadcast will wake up me and I'll recover myNode.Mu
	}
	myNode.Mu.Unlock()
}

func (myNode *Node) ReleaseCS(endpoints []string) {
	var wg sync.WaitGroup // used to count number of goroutines launched and wait for them to end properly
	myNode.Mu.Lock()
	myNode.RequestedSC = false
	myNode.Mu.Unlock()
	msg := Message{Id: myNode.Me, Reader: myNode.Reader, Seq: myNode.MySeq, Reply: true}
	for j, ep := range endpoints { // in myNode.Reply_deferred[me-1] is always false
		if myNode.Reply_deferred[j] { // notification to node j
			myNode.Mu.Lock()
			myNode.Reply_deferred[j] = false
			myNode.Mu.Unlock()
			wg.Add(1) // add one goroutine
			go func(ep string, m Message) {
				defer wg.Done()
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
	wg.Wait() // wait all goroutines to finish
}

func processReply(n *Node) {
	n.Mu.Lock()
	n.OutstandingReplies--
	if n.OutstandingReplies <= 0 {
		n.OutstandingReplies = 0
		n.AllReplied.Broadcast() // wake up the waiter
	}
	n.Mu.Unlock()
}

func processRequest(n *Node, msg Message, endpoints []string) {
	n.Mu.Lock()
	n.HighestSeq = Max(n.HighestSeq, msg.Seq)
	deferIt := n.RequestedSC && !(n.Reader && msg.Reader) &&
		((msg.Seq > n.MySeq) || (msg.Seq == n.MySeq && msg.Id > n.Me))
	n.Mu.Unlock()
	if deferIt {
		n.Mu.Lock()
		n.Reply_deferred[msg.Id-1] = true
		n.Mu.Unlock()
	} else { // then I REPLY to the REQUEST
		for {
			conn, err := net.Dial("tcp", endpoints[msg.Id-1])
			if err != nil {
				log.Println("Error connecting to ", endpoints[msg.Id-1], ":", err)
				time.Sleep(1 * time.Second)
				continue
			}
			if err = sendMsg(conn, Message{Id: n.Me, Reply: true}); err != nil {
				log.Println("Error messaging to ", endpoints[msg.Id-1], ":", err)
				time.Sleep(1 * time.Second)
				continue
			}
			conn.Close()
			break
		}
	}
}

// Called when a REPLY message arrives
// HandleReceivedMessages accepts connections and dispatches them.
// It listens until quit is closed; when quit is closed we close the listener
// which makes Accept return an error and the loop stops.
func HandleReceivedMessages(n *Node, endpoints []string, quit chan struct{}) {
	listener, err := net.Listen("tcp", endpoints[n.Me-1])
	CheckError(err)
	defer listener.Close()

	// goroutine to close listener when quit is signaled
	go func() {
		<-quit
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			// if listener closed due to quit, Accept returns an error; exit loop
			log.Println("Listener accept error (likely closed):", err)
			return
		}
		// handle the connection in a goroutine
		go func(c net.Conn) {
			defer c.Close()
			var msg Message
			if err := readMsg(c, &msg); err != nil {
				log.Println("Error reading message:", err)
				return
			}
			if msg.Reply {
				processReply(n)
			} else {
				processRequest(n, msg, endpoints)
			}
		}(conn)
	}
}

func main() {
	myNode := NewNode(2, true, 4)
	endpoints := []string{"127.0.0.1:29280", "127.0.0.1:29281", "127.0.0.1:29282", "127.0.0.1:29283"}
	log.SetFlags(log.Lshortfile | log.Lmicroseconds)
	quit := make(chan struct{})
	log.Println("Inicio a escuchar mensajes en Nodo: ", endpoints[myNode.Me-1])
	go HandleReceivedMessages(myNode, endpoints, quit)
	if myNode.Reader {
		log.Println("Nodo: ", myNode.Me, " Es un LECTOR")
	} else {
		log.Println("Nodo: ", myNode.Me, " Es un ESCRITOR")
	}
	log.Println("Nodo: ", myNode.Me, " va a pedir entrada en CS")
	// In this example I'm writer and I want to enter CS:
	myNode.RequestCS(endpoints)
	log.Println("Nodo: ", myNode.Me, " Ha conseguido entrar en CS ")
	time.Sleep(800 * time.Millisecond)
	// Now myNode is in SC
	// as this example is a writer could write and then notify new content to other writers
	// once finished I notify the rest
	myNode.ReleaseCS(endpoints)
	log.Println("Nodo: ", myNode.Me, "Ha salido de CS y la ha liberado para otros")
	time.Sleep(1000 * time.Millisecond)

	log.Println("Nodo: ", myNode.Me, " va a pedir entrada en CS por segunda vez")
	// In this example I'm writer and I want to enter CS:
	myNode.RequestCS(endpoints)
	log.Println("Nodo: ", myNode.Me, " Ha conseguido entrar en CS por segunda vez")
	time.Sleep(8000 * time.Millisecond)
	// Now myNode is in SC
	// as this example is a writer could write and then notify new content to other writers
	// once finished I notify the rest
	myNode.ReleaseCS(endpoints)
	log.Println("Nodo: ", myNode.Me, "Ha salido de CS y la ha liberado para otros")

	log.Println("Nodo: ", myNode.Me, " va a pedir entrada en CS por tercera vez")
	// In this example I'm writer and I want to enter CS:
	myNode.RequestCS(endpoints)
	log.Println("Nodo: ", myNode.Me, " Ha conseguido entrar en CS por tercera vez")
	time.Sleep(2000 * time.Millisecond)
	// Now myNode is in SC
	// as this example is a writer could write and then notify new content to other writers
	// once finished I notify the rest
	myNode.ReleaseCS(endpoints)
	log.Println("Nodo: ", myNode.Me, "Ha salido de CS y la ha liberado para otros")

	log.Println("Nodo: ", myNode.Me, " va a pedir entrada en CS por cuarta vez")
	// In this example I'm writer and I want to enter CS:
	myNode.RequestCS(endpoints)
	log.Println("Nodo: ", myNode.Me, " Ha conseguido entrar en CS por cuarta vez")
	time.Sleep(2000 * time.Millisecond)
	// Now myNode is in SC
	// as this example is a writer could write and then notify new content to other writers
	// once finished I notify the rest
	myNode.ReleaseCS(endpoints)
	log.Println("Nodo: ", myNode.Me, "Ha salido de CS y la ha liberado para otros")

	log.Println("Nodo: ", myNode.Me, " va a pedir entrada en CS por quinta vez")
	// In this example I'm writer and I want to enter CS:
	myNode.RequestCS(endpoints)
	log.Println("Nodo: ", myNode.Me, " Ha conseguido entrar en CS por quinta vez")
	time.Sleep(2000 * time.Millisecond)
	// Now myNode is in SC
	// as this example is a writer could write and then notify new content to other writers
	// once finished I notify the rest
	myNode.ReleaseCS(endpoints)
	log.Println("Nodo: ", myNode.Me, "Ha salido de CS y la ha liberado para otros")

	log.Println("Nodo: ", myNode.Me, " va a pedir entrada en CS por sexta vez")
	// In this example I'm writer and I want to enter CS:
	myNode.RequestCS(endpoints)
	log.Println("Nodo: ", myNode.Me, " Ha conseguido entrar en CS por sexta vez")
	time.Sleep(2000 * time.Millisecond)
	// Now myNode is in SC
	// as this example is a writer could write and then notify new content to other writers
	// once finished I notify the rest
	myNode.ReleaseCS(endpoints)
	log.Println("Nodo: ", myNode.Me, "Ha salido de CS y la ha liberado para otros")

	// Stop listener
	close(quit)
	// allow a small time to let graceful shutdown
	time.Sleep(100 * time.Millisecond)
}
