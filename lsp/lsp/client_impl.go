// Contains the implementation of a LSP client.

package lsp

import (
	//"container/list"
	"errors"
	"fmt"
	"encoding/json"
	"github.com/cmu440/lspnet"
	//"math"
	"time"
	_ "log"
)

type client struct {
    readPacChan  chan *packet
    sendPacChan  chan *packet
    writeChan    chan []byte
    readDataChan chan *Message
    closeChan    chan struct{}
    closeDone    chan bool
    params       *Params
    shutdownChan chan struct{}
    writeReplyChan chan bool
    connId       int

    epochCount int
    isClosed bool
    isConnLost bool
}

func newCli(param *Params) *client {
    return &client{
        readPacChan:  make(chan *packet),
        sendPacChan:  make(chan *packet),
        writeChan:    make(chan []byte),
        //readReqChan:  make(chan struct{}),
        readDataChan: make(chan *Message),
        closeChan:    make(chan struct{}),
        closeDone:    make(chan bool),
        params:       param,
        shutdownChan: make(chan struct{}),
        writeReplyChan: make(chan bool),
        epochCount:   0,
        isClosed:     false,
        isConnLost:     false,
    }
}

// NewClient creates, initiates, and returns a new client. This function
// should return after a connection with the server has been established
// (i.e., the client has received an Ack message from the server in response
// to its connection request), and should return a non-nil error if a
// connection could not be made (i.e., if after K epochs, the client still
// hasn't received an Ack message from the server in response to its K
// connection requests).
//
// hostport is a colon-separated string identifying the server's host address
// and port number (i.e., "localhost:9999").
func NewClient(hostport string, params *Params) (Client, error) {
	// get the udp address of the server.
	serverAddr, err := lspnet.ResolveUDPAddr("udp", hostport)
	if err != nil {
		return nil, err
	}
	//fmt.Println(serverAddr)
	// establish the connection.
	serverConn, err1 := lspnet.DialUDP("udp", nil, serverAddr)
	if err1 != nil {
		//fmt.Println("err1", err1)
		return nil, err1
	}

	// make a new client here.
	c := newCli(params)
	//fmt.Println("after init")
	// start the read and write goroutines, setup the epoch, send the connect
	// message and wait until the server replies ack, or the EpochLimit has
	// been reached.
	sendPacChanOut := make(chan *packet)
	readDataChanIn := make(chan *Message)

	// unbounded buffered channel between the main goroutine and 
	// the writeToConn goroutine.
	go handlePacBuffer(c.sendPacChan, sendPacChanOut, c.shutdownChan)
	// unbounded buffered channel between Read method and the
	// main goroutine.
	go handleMsgBuffer(readDataChanIn, c.readDataChan, c.shutdownChan)
	go receiveFromConn(serverConn, c.readPacChan)
	go writeToConn(serverConn, sendPacChanOut)

	connMsg := NewConnect()
	connPac := newPac(connMsg, nil)
	c.sendPacChan <- connPac
	epoch := time.NewTicker((time.Duration)(c.params.EpochMillis) * (time.Millisecond))


	var rdWin *readWindow
	var wtWin *writeWindow
	//fmt.Println("Before the connLoop")
connLoop:
	for {
		select {
		case <-epoch.C:
			c.epochCount = c.epochCount + 1
			if c.epochLimitExceeded() {
				err = errors.New("Not able to establish the connection.")
				c.shutdownAllChan(readDataChanIn)
				return nil, err
			}
			connPac := newPac(connMsg, nil)
			c.sendPacChan <- connPac
		case pac := <-c.readPacChan:
            //fmt.Println("*Message Read from chan", msg.String())
            //fmt.Println("here in connLoop", pac)
			if pac.message.Type == MsgAck && pac.message.SeqNum == 0 {
                //fmt.Println("I'm actually in!")
				c.connId = pac.message.ConnID
				// ReadWindow and WriteWindow for UDP
				rdWin = newReadWindow(c.params.WindowSize, c.sendPacChan, c.connId, serverAddr)
				wtWin = newWriteWindow(c.params.WindowSize, c.sendPacChan, c.connId, serverAddr)
				// the first ack to the server side.
				ackMsg := NewAck(c.connId, 0)
			    rdWin.ackQueue.PushBack(ackMsg)
			    ackPac := newPac(ackMsg, nil)
			    //fmt.Println("The server has replied.")
			    c.sendPacChan <- ackPac
				break connLoop
			}
		}
	}

	

    //fmt.Println("Connection established")
	// spin out a go routine that handles all channel information.
	go func() {
		for {
            //fmt.Println("really?")
			select {
			// Write request.
			case payload := <-c.writeChan:
				//fmt.Println(payload)
                //fmt.Println("here write")
				if !c.isConnLost {
					// send the content
					c.writeReplyChan <- true
					// slice the message into 1000 bytes slices.
					wtWin.sliceAndPush(payload)
				} else {
					c.writeReplyChan <- false
				}

			// epoch events happened.
			case <-epoch.C:
                //fmt.Println(c.epochCount)
				c.epochCount = c.epochCount + 1
				if c.epochLimitExceeded() {
					//fmt.Println("not hearing from the server, shutting down...")
					c.isConnLost = true
					c.shutdownAllChan(readDataChanIn)
					epoch.Stop()
					serverConn.Close()
				}

				if !c.isConnLost {
					// wtWin send all the messages in the window.
					wtWin.sendMsg()
					// send all the ack message in ackQueue
					for e := rdWin.ackQueue.Front(); e != nil; e = e.Next() {
						pac := newPac((e.Value).(*Message), nil)
						c.sendPacChan <- pac
					}
				}

			// close request.
			case <-c.closeChan:
                //fmt.Println("here close")
                c.isClosed = true
				rdWin.isClosed = true
				wtWin.isClosed = true

			// message arrives
			case pac := <-c.readPacChan:
                //fmt.Println(message.String())
                message := pac.message
				if message.ConnID != c.connId {
					continue
				}
				c.epochCount = 0
                //fmt.Println("[Read]", message)

				if message.Type == MsgAck {
					wtWin.acceptAckMsg(message)
				} else {
					rdWin.acceptDatMsg(message)
				}
			}

			// Close func is called, 
			if c.isClosed {
				//fmt.Println("[Client] in closed", c.isConnLost)
				// lost connection during the close procedure return false.
				if c.isConnLost {
					c.closeDone <- false
					return
				}
				// successfully closed.
				if wtWin.isSendComplete() {
					//fmt.Println("3")
					c.shutdownAllChan(readDataChanIn)
					epoch.Stop()
					serverConn.Close()
					c.closeDone <- true
					return
				}
				continue
			}

			if !c.isConnLost {
				msg := rdWin.getReadMsg()
				if msg != nil {
					readDataChanIn <- msg
				}
			}
		}
	}()

	return c, nil
}

func (c *client) ConnID() int {
	return c.connId
}

// same doubt with wirte
func (c *client) Read() ([]byte, error) {
	msg, ok := <-c.readDataChan	
	if !ok {
		//fmt.Println("readDataChan closed")
		err := errors.New("Lost connection")
		return nil, err
	}
	return msg.Payload, nil
}

// DOUBT: how could I return the right error for the Write
// function, since the error could happen both temporarily
// and spatially far away.
func (c *client) Write(payload []byte) error {
	//fmt.Println(payload)
	c.writeChan <- payload
	ok := <- c.writeReplyChan

	if !ok {
		return errors.New("Lost connection")
	}
	return nil
}

func (c *client) Close() error {
	//fmt.Println("Closed called")
	c.closeChan <- struct{}{}
	ok := <-c.closeDone
	if !ok {
		//fmt.Println("Lost Connection when closing.")
	} else {
		//fmt.Println("Successfully closed.")
	}
	return nil
}

func (c *client) epochLimitExceeded() bool {
	if c.epochCount > c.params.EpochLimit {
		return true
	}
	return false
}

// called when connection is lost, shut down write, read, sendpak channel,
// so Write(), Read() func call would return immediately, and writeToConn
// go routine would stops.
func (c *client) shutdownAllChan(readDataChanIn chan *Message) {
    //fmt.Println("In shutdownAllChan func")
    close(readDataChanIn)
    close(c.sendPacChan)
    close(c.shutdownChan)
    //fmt.Println("after close channel in shutdownAllChan")
}

func writeToConn(conn *lspnet.UDPConn, sendPckChan chan *packet) {
    for {
        packet, ok := <-sendPckChan
        if !ok {
        	// the channel has been closed by main goroutine.
        	return
        }
        //fmt.Println("Payload Before Marshalling", message,len(message.Payload))
        //fmt.Println("In writeToConn", message.String())
        //fmt.Println(packet.message)
        bytes, err := json.Marshal(packet.message)

        //fmt.Println("After Marshalling", string(bytes), len(bytes))

        if err != nil {
            fmt.Println("not ok marshalling")
        }
        _, err = conn.Write([]byte(bytes))
        if err != nil {
            //fmt.Println(err, "cannot write to the connection!")
            return
        }
        //fmt.Println("[write]", n, "bytes sent")
    }
}

func receiveFromConn(conn *lspnet.UDPConn, readPacChan chan *packet) {
    for {
    	/*select {
    	case <-shutdownChan:

    	}*/
        buf := make([]byte, 2000)
        n, err := conn.Read(buf[0:])
        //fmt.Println("[Read]", buf[:n], n)
        if err != nil {
            //fmt.Println("[Client] ", err, ", server listen UDPConn has corrupted.")
            time.Sleep(200 * time.Millisecond)
            continue
            //return
        }
        message := Message{}
        err = json.Unmarshal(buf[:n], &message)

        //log.Printf("[Read] %s", message)
        if err != nil {
            fmt.Println("Unable to unmarshal the bytes")
        }
        //fmt.Println("[Client] Read reasonable message", message)

        pac := newPac(&message, nil)
        readPacChan <- pac
    }
}