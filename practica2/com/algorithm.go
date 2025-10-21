package com

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

func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
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
