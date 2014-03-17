package lsp

import (
    "container/list"
    //"encoding/json"
    //"errors"
    "fmt"
    "github.com/cmu440/lspnet"
    "math"
    //"log"
)

// packet wraps the message and the udpaddr associated with
// this message.
type packet struct {
    message *Message
    addr    *lspnet.UDPAddr
}

func newPac(msg *Message, address *lspnet.UDPAddr) *packet {
    return &packet{
        message: msg,
        addr:    address,
    }
}

type readWindow struct {
    // this channel is used to send write messages
    sendPacChan chan *packet

    // window size
    size int

    // ack queue that is being sent to the server
    ackQueue *list.List

    // messages being read
    msgInReading *list.List

    // messages with SeqNum <= this number has already been received
    readSafeNum int

    // connection Id associate with this read window
    connId int

    // udp address of the connection
    addr *lspnet.UDPAddr

    // connection is being closed, refuse to send acks for new comming messages.
    isClosed bool
}

func newReadWindow(s int, ch chan *packet, id int, udpAddr *lspnet.UDPAddr) *readWindow {
    return &readWindow{
        sendPacChan:  ch,
        size:         s,
        ackQueue:     list.New(),
        msgInReading: list.New(),
        readSafeNum:  0,
        connId:       id,
        addr:         udpAddr,
        isClosed:     false,
    }
}

// if the msgInReading is not nil and its first element's seq num is smaller
// or equal to the readSafeNum, pop the first element and return it. otherwise
// return nil.
func (win *readWindow) getReadMsg() *Message {
    if win.msgInReading.Len() == 0 || win.isClosed {
        return nil
    }
    //e := win.msgInReading.Front()
    //fmt.Println("[In getReadMsg]", e.Value.(*Message))
    if e := win.msgInReading.Front(); e.Value.(*Message).SeqNum <= win.readSafeNum {
        win.msgInReading.Remove(e)
        //fmt.Println("[e]", e.Value.(*Message))
        return e.Value.(*Message)
    } 
    return nil
}

func (win *readWindow) acceptConMsg(con *Message) {
    win.readSafeNum = 0
    ackMsg := NewAck(win.connId, 0)
    win.ackQueue.PushBack(ackMsg)
    ackPac := newPac(ackMsg, win.addr)
    //fmt.Println("The window's addr:", win.addr)
    win.sendPacChan <- ackPac
}

// message reached. check the integrity of this message, remove from
// ackQueue the unnecessary ones. book-keep the ackMsg, send the ackMsg,
// store the message, update the readSafeNum.
func (win *readWindow) acceptDatMsg(dat *Message) {

    if dat.SeqNum <= win.readSafeNum {
        //fmt.Println("[readWindow]", dat.String(), "readSafeNum: ", win.readSafeNum)
        // the message's seqnum is below what we want.
        return
    } else {
        // remove from ackQueue that is already out of the writing window in the other
        // side.
        //fmt.Println("[readWindow]", dat.String())
        ackAlreadyIn := false
        for e := win.ackQueue.Front(); e != nil; {
            next := e.Next()
            if e.Value.(*Message).SeqNum <= dat.SeqNum - win.size {
                win.ackQueue.Remove(e)
            }
            if e.Value.(*Message).SeqNum == dat.SeqNum {
                ackAlreadyIn = true
            }
            e = next
        }

        // the ack message is already there, we've received this dat before.
        if ackAlreadyIn {
            return
        }

        // The readSafeNum+1 message is missing or the readWindow is fixed because of
        // being closed.
        if dat.SeqNum > win.readSafeNum + win.size {
            if !win.isClosed {
                fmt.Println("[readWindow]Wrong ackMsg read in client, the server send way bebind data message")
            }
            return
        }

        // add this ack into ackQueue if not already inside it.
        ackMsg := NewAck(win.connId, dat.SeqNum)
        win.ackQueue.PushBack(ackMsg)
        ackPac := newPac(ackMsg, win.addr)
        win.sendPacChan <- ackPac
    }

    // if this is being closed, so no more message is added into logack
    if win.isClosed {
        return
    }

    // store this message for later read. insert message into msgInReading.
    // msgInReading is kept in order.
    for e := win.msgInReading.Back(); ; e = e.Prev() {
        if e == nil {
            win.msgInReading.PushFront(dat)
            break
        }
        if (e.Value).(*Message).SeqNum < dat.SeqNum {
            win.msgInReading.InsertAfter(dat, e)
            break
        } else if (e.Value).(*Message).SeqNum == dat.SeqNum {
            fmt.Println("The program should never come here.")
            break
        }
    }

    // check if we can move the readSafeNum
    for e := win.msgInReading.Front(); e != nil; {
        next := e.Next()
        if (e.Value).(*Message).SeqNum == win.readSafeNum+1 {
            win.readSafeNum = win.readSafeNum+1
        } 
        //else if (e.Value).(*Message).SeqNum < win.readSafeNum+1 {
        //    win.msgInReading.Remove(e)
        //    fmt.Println("Notice! The logic flow has been compromised for msgInReading")
        //}
        e = next
    }
    //fmt.Println("Stored one message in readWindow, msgInReading len", win.msgInReading.Len(), win.readSafeNum)

}

// when all Ack message have been received by the other side, return true.
func (win *readWindow) isAckComplete() bool {
    return (win.ackQueue.Len() == 0)
}

type writeWindow struct {
    // this channel is used to send write messages
    sendPacChan chan *packet

    // data messages that is being or going to be sent
    msgSendQueue *list.List

    // acked number of messages (<= number are all acked)
    sendCompleteNum int

    // messages in current window which is already received by the other side
    msgSent *list.List

    // window size
    size int

    // the seq number that is assgined to next writing content
    sendSeqNum int

    // connection id associate with this window
    connId int

    // udp address
    addr *lspnet.UDPAddr

    // isClosed
    isClosed bool
}

func newWriteWindow(s int, ch chan *packet, id int, udpAddr *lspnet.UDPAddr) *writeWindow {
    return &writeWindow{
        sendPacChan:     ch,
        msgSendQueue:    list.New(),
        sendCompleteNum: 0,
        msgSent:         list.New(),
        size:            s,
        sendSeqNum:      1,
        connId:          id,
        addr:            udpAddr,
        isClosed:        false,
    }
}

// slice the load into 1000 bytes slices, push them into msgSendQueue of win.
// increase the sendSeqNum and return current sendSeqNum.
func (win *writeWindow) sliceAndPush(load []byte) {
    //fmt.Println("connId", connId, "sendSeqNum", ind)
    if win.isClosed {
        return
    }
    readBytes := 0
    remainBytes := len(load)
    var message *Message
    for remainBytes > 0 {
        if (remainBytes > 1000) {
            message = NewData(win.connId, win.sendSeqNum, load[readBytes:readBytes+1000])
            readBytes += 1000
            remainBytes -= 1000
        } else {
            message = NewData(win.connId, win.sendSeqNum, load[readBytes:])
            remainBytes = 0
        }
        win.sendSeqNum = win.sendSeqNum + 1
        win.addMsgToSendQueue(message)
    }
}

func (win *writeWindow) addMsgToSendQueue(msg *Message) {
    win.msgSendQueue.PushBack(msg)
    if (msg.SeqNum <= win.sendCompleteNum+win.size) {
        //fmt.Println("msgSeq", msg.SeqNum, "winsize", win.size)
        pac := newPac(msg, win.addr)
        win.sendPacChan <- pac
        //fmt.Println("after blocking")
    }
}

// this function is called when a epoch event happens. It would check
// if it could get anything from msgSendQueue to send.
func (win *writeWindow) sendMsg() {
    if win.msgSendQueue.Len() == 0 {
        return
    }
    for e := win.msgSendQueue.Front(); e != nil; e = e.Next() {
        if msg := e.Value.(*Message); msg.SeqNum <= win.sendCompleteNum+win.size {
            pac := newPac(msg, win.addr)
            win.sendPacChan <- pac
        } else {
            break
        }
    }
}

// returns true if the window has received acks for all its sending messages.
func (win *writeWindow) isSendComplete() bool {
    return win.msgSendQueue.Len() == 0
}

// if this is an useful ackMsg, find in the msgInSending if there is any
// msg can be acked. if there is, remove the acked one. then renew
// the sendCompleteNum and send messages that become part of the window.
func (win *writeWindow) acceptAckMsg(ack *Message) {
    // if seqNum is what we have already confirmed.
    if ack.SeqNum <= win.sendCompleteNum {
        return
    }

    // we can't send msg exceeding the window, so they can't receive such msgs
    if ack.SeqNum > win.sendCompleteNum+win.size {
        fmt.Println("[writeWindow]The ackMsg sent has a wrong SeqNum")
    }

    // remove the message in the sendQueue correspoding to this ack.
    // add it into the msgSent
    for e := win.msgSendQueue.Front(); e != nil; {
        next := e.Next()

        if message := (e.Value).(*Message); ack.SeqNum == message.SeqNum {
            win.msgSendQueue.Remove(e)
            win.msgSent.PushBack(message)
            break
        }
        e = next
    }

    // if this ack is not the one that restrict the write window.
    if ack.SeqNum != win.sendCompleteNum + 1 {
        return
    }

    // update sendCompleteNum
    // if the msgSendQueue is empty, which means all msgs have been sent
    // then the sendCompleteNum is simply the largest number in the msgSent
    if win.msgSendQueue.Len() == 0 {
        max := 0
        for e := win.msgSent.Front(); e != nil; e = e.Next() {
            max = (int)(math.Max((float64)(max), (float64)((e.Value).(*Message).SeqNum)))
        }
        win.sendCompleteNum = max
        win.msgSent.Init()
    } else {
        // if the msgSendQueue is not empty, pick the smallest one's
        // seqnum-1 as the sendCompleteNum. Meanwhile, send the messages
        // that become part of the window

        oldWinEnd := win.sendCompleteNum + win.size

        win.sendCompleteNum = (win.msgSendQueue.Front().Value).(*Message).SeqNum - 1
        // remove the msgs below sendCompleteNum in msgSent
        for e := win.msgSent.Front(); e != nil; {
            next := e.Next()
            if (e.Value).(*Message).SeqNum <= win.sendCompleteNum {
                win.msgSent.Remove(e)
            }
            e = next
        }

        newWinEnd := win.sendCompleteNum + win.size

        // send those msgs which newly entered the write window.
        for e := win.msgSendQueue.Front(); e != nil && e.Value.(*Message).SeqNum <= newWinEnd; e = e.Next() {
            if message := e.Value.(*Message); message.SeqNum > oldWinEnd {
                pac := newPac(message, win.addr)
                win.sendPacChan <- pac
            }
        }
    }
}

func handleMsgBuffer(in <-chan *Message, out chan<- *Message, shutdownChan chan struct{}) {
    defer close(out)

    buffer := list.New()

    for {
        if buffer.Len() == 0 {
            v, ok := <-in
            //fmt.Println(v.String())
            if !ok {
                flushMsg(buffer, out)
                return
            }
            buffer.PushBack(v)
        }
        select {
        case v, ok := <-in:
            //fmt.Println(v.String())
            if !ok {
                //flushMsg(buffer, out)
                return
            }
            buffer.PushBack(v)
        case out <- (buffer.Front().Value).(*Message):
            buffer.Remove(buffer.Front())
        case <-shutdownChan:
            //fmt.Println("really closing?")
            return
        }
    }
}

func flushMsg(buffer *list.List, out chan<- *Message) {
    for e := buffer.Front(); e != nil; e = e.Next() {
        out <- (e.Value).(*Message)
    }
}

func handlePacBuffer(in <-chan *packet, out chan<- *packet, shutdownChan chan struct{}) {
    defer close(out)

    buffer := list.New()

    for {
        if buffer.Len() == 0 {
            v, ok := <-in
            //fmt.Println(v.String())
            if !ok {
                flushPac(buffer, out)
                return
            }
            buffer.PushBack(v)
        }
        select {
        case v, ok := <-in:
            //fmt.Println(v.String())
            if !ok {
                //flushPac(buffer, out)
                return
            }
            buffer.PushBack(v)
        case out <- (buffer.Front().Value).(*packet):
            buffer.Remove(buffer.Front())
        case <-shutdownChan:
            return
        }
    }
}

func flushPac(buffer *list.List, out chan<- *packet) {
    for e := buffer.Front(); e != nil; e = e.Next() {
        out <- (e.Value).(*packet)
    }
}

func handleRdContentBuffer(in <-chan *readContent, out chan<- *readContent, shutdownChan chan struct{}) {
    defer close(out)

    buffer := list.New()

    for {
        if buffer.Len() == 0 {
            v, ok := <-in
            //fmt.Println(v.String())
            if !ok {
                flushRdContent(buffer, out)
                return
            }
            buffer.PushBack(v)
        }
        select {
        case v, ok := <-in:
            //fmt.Println(v.String())
            if !ok {
                //flushRdContent(buffer, out)
                return
            }
            buffer.PushBack(v)
        case out <- (buffer.Front().Value).(*readContent):
            buffer.Remove(buffer.Front())
        case <-shutdownChan:
            return
        }
    }
}

func flushRdContent(buffer *list.List, out chan<- *readContent) {
    for e := buffer.Front(); e != nil; e = e.Next() {
        out <- (e.Value).(*readContent)
    }
}