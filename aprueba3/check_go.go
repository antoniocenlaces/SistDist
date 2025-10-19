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
	Reply  bool // true is a message replying my REQUEST
}

type Node struct {
	mu                 sync.Mutex   // to control exclusive access to common variables
	me                 int          // my own node number
	outstandingReplies int          // number of REPLY still ot received
	allReplied         *sync.Cond   // condition to chek if all have sent their REPLY
	mySeq              int          // my own squence number to be sent with my REQUEST
	highestSeq         int          // highest sequence numbe I have seen in any REQUEST sent or received
	requestedSC        bool         // to control when I have requested to enter CS
	reader             bool         //true is a reader; false is a writer;
	reply_deferred     map[int]bool // dictionary with node numbers that I have deferred their REQUEST
	totalNodes         int
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func NewNode(me int, function bool, totalNodes int) *Node {
	n := &Node{}
	n.me = me
	n.totalNodes = totalNodes
	n.allReplied = sync.NewCond(&n.mu)
	n.outstandingReplies = 0
	n.mySeq = 0
	n.highestSeq = 0
	n.requestedSC = false
	n.reader = function
	n.reply_deferred = make(map[int]bool, totalNodes)
	return n
}

func (myNode *Node) requestCS(endpoints []string) {
	// Using myNode mutex common variables are updated to REQUEST critical section
	myNode.mu.Lock()
	myNode.requestedSC = true
	myNode.mySeq = myNode.highestSeq + 1
	myNode.outstandingReplies = myNode.totalNodes - 1
	myNode.mu.Unlock()
	msg := Message{Id: myNode.me, Reader: myNode.reader, Seq: myNode.mySeq, Reply: false}
	// a message with my Id, my function (reader or writer) and my actual sequence number are sent to all other nodes
	for i, ep := range endpoints {
		if i+1 != myNode.me {
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
	myNode.mu.Lock()
	for myNode.outstandingReplies > 0 {
		myNode.allReplied.Wait() // I go to sleep while all other sent their notification
		// a Broadcast will wake up me and I'll recover myNode.mu
	}
	myNode.mu.Unlock()
}

func (myNode *Node) releaseCS(endpoints []string) {
	var wg sync.WaitGroup // used to count number of goroutines launched and wait for them to end properly
	myNode.mu.Lock()
	myNode.requestedSC = false
	myNode.mu.Unlock()
	msg := Message{Id: myNode.me, Reader: myNode.reader, Seq: myNode.mySeq, Reply: true}
	for j, ep := range endpoints { // in myNode.reply_deferred[me-1] is always false
		if myNode.reply_deferred[j] { // notification to node j
			myNode.mu.Lock()
			myNode.reply_deferred[j] = false
			myNode.mu.Unlock()
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
	n.mu.Lock()
	n.outstandingReplies--
	if n.outstandingReplies <= 0 {
		n.outstandingReplies = 0
		n.allReplied.Broadcast() // wake up the waiter
	}
	n.mu.Unlock()
}

func processRequest(n *Node, msg Message, endpoints []string) {
	n.mu.Lock()
	n.highestSeq = max(n.highestSeq, msg.Seq)
	deferIt := n.requestedSC && ((msg.Seq > n.mySeq) || (msg.Seq == n.mySeq && msg.Id > n.me))
	n.mu.Unlock()
	if deferIt {
		n.reply_deferred[msg.Id-1] = true
	} else { // then I REPLY to the REQUEST
		for {
			conn, err := net.Dial("tcp", endpoints[msg.Id-1])
			if err != nil {
				log.Println("Error connecting to ", endpoints[msg.Id-1], ":", err)
				time.Sleep(1 * time.Second)
				continue
			}
			if err = sendMsg(conn, Message{Id: n.me, Reply: true}); err != nil {
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
// handleReceivedMessages accepts connections and dispatches them.
// It listens until quit is closed; when quit is closed we close the listener
// which makes Accept return an error and the loop stops.
func handleReceivedMessages(n *Node, endpoints []string, quit chan struct{}) {
	listener, err := net.Listen("tcp", endpoints[n.me-1])
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
	myNode := NewNode(2, true, 2)
	endpoints := []string{"127.0.0.1:29280", "127.0.0.1:29281"}
	log.SetFlags(log.Lshortfile | log.Lmicroseconds)
	quit := make(chan struct{})
	fmt.Println("Inicio a escuchar mensajes en ", endpoints[myNode.me-1])
	go handleReceivedMessages(myNode, endpoints, quit)
	// In this example I'm writer and I want to enter CS:
	myNode.requestCS(endpoints)
	fmt.Println("He conseguido entrar en CS ")
	time.Sleep(500 * time.Millisecond)
	// Now myNode is in SC
	// as this example is a writer could write and then notify new content to other writers
	// once finished I notify the rest
	myNode.releaseCS(endpoints)
	fmt.Println("He salido de CS y la he liberado para otros")
	// Stop listener
	close(quit)
	// allow a small time to let graceful shutdown
	time.Sleep(100 * time.Millisecond)
}
