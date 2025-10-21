package com

import (
	"sync"
)

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
