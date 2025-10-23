package ra

import (
	"os"
	"testing"
	"time"
)

func createTempPeersFile(content string) string {
	tmpfile, _ := os.CreateTemp("", "peers*.txt")
	tmpfile.WriteString(content)
	tmpfile.Close()
	return tmpfile.Name()
}

func TestRAInitialization(t *testing.T) {
	path := createTempPeersFile("localhost:8001\nlocalhost:8002\n")
	ra := New(1, path, true)
	if ra.me != 1 || ra.totalNodes != 2 || ra.Reader != true {
		t.Errorf("RA initialization failed: %+v", ra)
	}
	ra.Stop()
}

func TestRAReaderWriterInteraction(t *testing.T) {
	path := createTempPeersFile("localhost:8003\nlocalhost:8004\n")
	reader := New(1, path, true)
	writer := New(2, path, false)

	time.Sleep(100 * time.Millisecond)

	go reader.PreProtocol()
	go writer.PreProtocol()

	time.Sleep(500 * time.Millisecond)

	reader.PostProtocol()
	writer.PostProtocol()

	reader.Stop()
	writer.Stop()
}

func TestRADeferredReplies(t *testing.T) {
	path := createTempPeersFile("localhost:8005\nlocalhost:8006\n")
	node1 := New(1, path, false)
	node2 := New(2, path, false)

	time.Sleep(100 * time.Millisecond)

	go node1.PreProtocol()
	time.Sleep(100 * time.Millisecond)
	go node2.PreProtocol()

	time.Sleep(500 * time.Millisecond)

	node1.PostProtocol()
	node2.PostProtocol()

	node1.Stop()
	node2.Stop()
}
