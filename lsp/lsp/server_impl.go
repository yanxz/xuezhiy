// Contains the implementation of a LSP server.

package lsp

import (
	"strconv"
	"errors"
	"github.com/cmu440/lspnet"
	"time"
	"fmt"
	"encoding/json"
)

type server struct {
	// same as client but the readWinowMap and writeWindowMap
	readPacChan  chan *packet
    sendPacChan  chan *packet
    writeChan    chan *writeContent
    readDataChan chan *readContent
    closeCliChan chan int
    closeCliReplyChan chan bool
    closeSerChan chan struct{}
    closeSerReplyChan chan bool
    params       *Params
    shutdownChan chan struct{}
    writeReplyChan chan bool

    // serDownChan  chan struct{}
    addrConnMap  map[string](*clientConn)
    idConnMap	 map[int](*clientConn)
    connCount    int
    isSerClosed     bool
    isCloseSuccess bool
}

// struct that represents one client connection.
type clientConn struct {
	addr *lspnet.UDPAddr
	connId int
	rdWin *readWindow
	wtWin *writeWindow
	epoch int
	isClosed bool
	isConnLost bool
	hasReportedConnLost bool
}

func newSer(param *Params) *server {
	return &server{
		readPacChan:  make(chan *packet),
	    sendPacChan:  make(chan *packet),
	    writeChan:    make(chan *writeContent),
	    readDataChan: make(chan *readContent),
	    closeCliChan: make(chan int),
	    closeCliReplyChan: make(chan bool),
	    closeSerChan: make(chan struct{}),
        closeSerReplyChan: make(chan bool),

	    params:       param,
	    shutdownChan: make(chan struct{}),
	    writeReplyChan: make(chan bool),
	    
	    addrConnMap:  make(map[string]*clientConn),
	    idConnMap:  make(map[int]*clientConn),
	    connCount:    1,
	    isSerClosed:     false,
	    isCloseSuccess: true,
	    // keep this map even after the connid is lost. to
	    // avoid race conditions.
	}
}

// NewServer creates, initiates, and returns a new server. This function should
// NOT block. Instead, it should spawn one or more goroutines (to handle things
// like accepting incoming client connections, triggering epoch events at
// fixed intervals, synchronizing events using a for-select loop like you saw in
// project 0, etc.) and immediately return. It should return a non-nil error if
// there was an error resolving or listening on the specified port number.
func NewServer(port int, params *Params) (Server, error) {
	// begin to listen
	addr, err := lspnet.ResolveUDPAddr("udp", lspnet.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	
	udpConn, err1 := lspnet.ListenUDP("udp", addr)
	if err1 != nil {
		return nil, err1
	}

	s := newSer(params)

	sendPacChanOut := make(chan *packet)
	readDataChanIn := make(chan *readContent)
	go handlePacBuffer(s.sendPacChan, sendPacChanOut, s.shutdownChan)
	go handleRdContentBuffer(readDataChanIn, s.readDataChan, s.shutdownChan)
	go receiveFromConnUDP(udpConn, s.readPacChan)
	go writeToConnUDP(udpConn, sendPacChanOut)

	epoch := time.NewTicker((time.Duration)(s.params.EpochMillis) * (time.Millisecond))

	go func() {
		for {
			select {
			case content := <-s.writeChan:
				conn, exist := s.idConnMap[content.connId]
				if !exist {
					// the connection is already closed
					s.writeReplyChan <- false
					//fmt.Println("[Server]Notice! Write Call on closed connection")
					continue
				}

				if s.isSerClosed {
					//fmt.Println("Write call on closed server")
					continue
				}

				if !conn.isConnLost {
					// send the content
					s.writeReplyChan <- true
					conn.wtWin.sliceAndPush(content.payload)
				} else {
					s.writeReplyChan <- false
				}

			case <-epoch.C:
				for _, conn := range s.idConnMap {
					conn.epoch = conn.epoch + 1
					if conn.epoch == s.params.EpochLimit + 1 {
						//fmt.Printf("Client %d has died...\n", connid)
						conn.isConnLost = true
					}
				}

				for _, conn := range s.idConnMap {
					conn.wtWin.sendMsg()
					for e := conn.rdWin.ackQueue.Front(); e != nil; e = e.Next() {
						pac := newPac((e.Value).(*Message), conn.addr)
						s.sendPacChan <- pac
					}
				}

			case pac := <-s.readPacChan:
				if pac == nil {
					continue
				}
				theAddr := pac.addr
				message := pac.message
				conn, ok := s.addrConnMap[theAddr.String()]
				//fmt.Println("[Read]", message.String(), " ", len([]byte(theAddr)))
				// a message from an unknown address when the server is not closed.
				if !ok && !s.isSerClosed {
					if message.Type != MsgConnect || message.SeqNum != 0 ||
						message.ConnID != 0 || message.Payload != nil {
							continue
					}
					// add this new connection
					s.newConnect(theAddr)
					newConn := s.addrConnMap[theAddr.String()]
					//fmt.Println("The first time registered:", len([]byte(theAddr)))

					// send back the ack, also register it in the ackqueue
					newConn.rdWin.acceptConMsg(message)
					//fmt.Println("[writeToConn]", packet.message, packet.addr)
			        continue
				}

				// refresh the epoch number.
				conn.epoch = 0

				// from known address
				if message.Type == MsgAck {
					conn.wtWin.acceptAckMsg(message)

				} else if message.Type == MsgData {
					conn.rdWin.acceptDatMsg(message)
				}

			case connid := <-s.closeCliChan:
				conn, ok := s.idConnMap[connid]
				if !ok {
					s.closeCliReplyChan <- false
					continue
				} else if conn.isClosed {
					//fmt.Println("[Server] close on already closed channel")
					s.closeCliReplyChan <- false
					continue
				}
				s.closeCliReplyChan <- true
				// mark the conn as closed
				conn.isClosed = true
				conn.rdWin.isClosed = true
				conn.wtWin.isClosed = true

				// immediately send the error message to readDataChan
				s := fmt.Sprintf("Connection of %d has been lost and no messages left", connid)
				e := errors.New(s)
				rdContent := newRdContent(connid, nil, e)

				readDataChanIn <- rdContent

			case <-s.closeSerChan:
				s.isSerClosed = true
				for _, conn := range s.idConnMap {
					conn.isClosed = true
					conn.rdWin.isClosed = true
					conn.wtWin.isClosed = true
				}
			}

			if s.isSerClosed {
				// all clientConn has been removed from the client.
				if len(s.idConnMap) == 0 {
					s.closeSerReplyChan <- s.isCloseSuccess

					// close all background goroutines
					s.shutdownAllChan()
					epoch.Stop()
					udpConn.Close()
					return
				}
				for _, conn := range s.idConnMap {
					if conn.isConnLost && !conn.wtWin.isSendComplete() {
						s.isCloseSuccess = false
					}
				}
			}

			// send msg if there is.
			for _, conn := range s.idConnMap {
				msg := conn.rdWin.getReadMsg()
				if msg != nil {
					rdContent := newRdContent(conn.connId, msg.Payload, nil)
					readDataChanIn <- rdContent
				} else if conn.isConnLost && !conn.hasReportedConnLost {
					s := fmt.Sprintf("Connection of %d has been lost and no messages left", conn.connId)
					e := errors.New(s)
					rdContent := newRdContent(conn.connId, nil, e)
					conn.hasReportedConnLost = true
					readDataChanIn <- rdContent
				}
			}

			// check if any closed connection could be deleted from the server
			for _, conn := range s.idConnMap {
				if conn.isClosed {
					// connection already lost or there are no pending message
					if conn.isConnLost || conn.wtWin.isSendComplete() {
						s.remove(conn.connId)
					}
				}
			}
		}
	}()

	return s, nil
}

type readContent struct {
	connId int
	payload []byte
	err error
}

func newRdContent(connid int, load []byte, e error) *readContent {
	return &readContent {
		connId: connid,
		payload: load,
		err: e,
	}
}

func (s *server) Read() (int, []byte, error) {
	rdRes, ok := <-s.readDataChan
	if !ok {
		err := errors.New("Server has been closed")
		//fmt.Println(err)
		return -1, nil, err
	}

	if rdRes.err != nil {
		return rdRes.connId, nil, rdRes.err
	}
	
	return rdRes.connId, rdRes.payload, rdRes.err
}

// wrapper of contents sending from write function to main goroutine.
// consisting of connid and payload.
type writeContent struct{
	connId int
	payload []byte
}

func newWtContent(connid int, load []byte) *writeContent {
	return &writeContent {
		connId: connid,
		payload: load,
	}
}

func (s *server) shutdownAllChan() {
	close(s.shutdownChan)
}

func (s *server) Write(connid int, payload []byte) error {
	content := newWtContent(connid, payload)
	s.writeChan <- content

	exist := <- s.writeReplyChan
	if !exist {
		err := fmt.Sprintf("Lost connction to %d client", connid)
		return errors.New(err)
	} else {
		return nil
	}
}

func (s *server) CloseConn(connid int) error {
	s.closeCliChan <- connid
	ok := <- s.closeCliReplyChan
	if !ok {
		return errors.New("closeChan on unknown Id")
	}
	return nil
}

func (s *server) Close() error {
	s.closeSerChan <- struct{}{}
	ok := <- s.closeSerReplyChan
	if !ok {
		err := errors.New("Lost connection during closing")
		return err
	}
	return nil
}

func (s *server) newConnect(newAddr *lspnet.UDPAddr) {
	rdWindow := newReadWindow(s.params.WindowSize, s.sendPacChan, s.connCount, newAddr)
	rdWindow.readSafeNum = -1
	wtWindow := newWriteWindow(s.params.WindowSize, s.sendPacChan, s.connCount, newAddr)
	wtWindow.sendSeqNum = 1
	wtWindow.sendCompleteNum = 0
	conn := 
		&clientConn {
			connId: s.connCount,
			addr: newAddr,
			rdWin: rdWindow,
			wtWin: wtWindow,
			epoch: 0,
			isClosed: false,
			isConnLost: false,
			hasReportedConnLost: false,
		}
	s.idConnMap[s.connCount] = conn
	s.addrConnMap[newAddr.String()] = conn
	//fmt.Printf("client %d connected in\n", s.connCount)
	s.connCount = s.connCount + 1
}

func (s *server) remove(connid int) {
	conn := s.idConnMap[connid]
	delete(s.idConnMap, connid)
	delete(s.addrConnMap, conn.addr.String())
}

func writeToConnUDP(conn *lspnet.UDPConn, sendPacChan chan *packet) {
    for {
        packet := <-sendPacChan
        //fmt.Println("Payload Before Marshalling", message,len(message.Payload))
        //fmt.Println("[writeToConn]", packet.message, packet.addr)
        bytes, err := json.Marshal(packet.message)

        //fmt.Println("After Marshalling", string(bytes))

        if err != nil {
            fmt.Println("not ok marshalling")
        }
        //fmt.Println(packet.addr)
        _, err = conn.WriteToUDP([]byte(bytes), packet.addr)
        if err != nil {
            //fmt.Println(err, "cannot write to the connection!")
            return
        }
        //fmt.Println("[write]", n, "bytes sent")
    }
}

func receiveFromConnUDP(conn *lspnet.UDPConn, readPacChan chan *packet) {
    for {
        buf := make([]byte, 2000)
        n, addr, err := conn.ReadFromUDP(buf[0:])
        //fmt.Println("[Read]", buf[:n], )
        if err != nil {
            //fmt.Printf("[Server]", err)
            return
        }
        message := Message{}
        err = json.Unmarshal(buf[:n], &message)

        //fmt.Println("[Read]", message.String())
        if err != nil {
            fmt.Println("Unable to unmarshal the bytes")
        }
        //fmt.Println("[Server] Read reasonable message", message)

        pac := newPac(&message, addr)
        readPacChan <- pac
    }
}