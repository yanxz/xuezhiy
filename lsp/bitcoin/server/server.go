package main

import (
	"fmt"
	"os"
	"github.com/cmu440/lsp"
	//"lspnet"
	"log"
	"encoding/json"
	"strconv"
	"math"
	//"errors"
	"container/list"
	"github.com/cmu440/bitcoin"
)

type myServer struct {
	// the lsp.Server
	ser *lsp.Server

	// mapping client's connid to the job they submit.
	connJobMap map[int](*job)

	// miner maps. recording every miner this server has.
	minerMap map[int]bool

	// mapping miner's connid to the jobChunk they are handling.
	minerJobMap map[int](*jobChunk)

	// queue of client requests telling the order of process.
	jobQueue *list.List

	// channel for join from miner.
	joinChan chan int

	// channel for request from client.
	reqChan chan *reqresPacket

	// channel for partial result from miner.
	resultChan chan *reqresPacket

	// channel for error from miner.
	errChan chan int

	// a list of available miners.
	idles *list.List
}

func newMyServer(server *lsp.Server) *myServer {
	return &myServer {
		ser: server,
		connJobMap: make(map[int](*job)),
		minerMap: make(map[int]bool),
		minerJobMap: make(map[int](*jobChunk)),
		jobQueue: list.New(),
		joinChan: make(chan int),
		reqChan: make(chan *reqresPacket),
		resultChan: make(chan *reqresPacket),
		errChan: make(chan int),
		idles: list.New(),
	}
}

type job struct {
	// the cliend connid of this job.
	clientId int

	// the list of jobChunks that needed to be worked out.
	jobChunks *list.List

	// the string.
	data string

	// current minimum hash value.
	hashVal uint64

	// current nonce for the hashVal.
	nonce uint64

	// the next jobChunk that should be worked out in the jobChunks.
	next *jobChunk
}

func createNewJob(id int, reqMsg *bitcoin.Message) *job {
	upper := reqMsg.Upper
	lower := reqMsg.Lower

	jobChunkList := list.New()

	done := lower
	remaining := upper-lower+1
	var chunk *jobChunk
	for remaining > 0 {
		if remaining > chunksize {
			chunk = newJobChunk(id, done, done+chunksize-1, reqMsg.Data)
			remaining -= chunksize
			done += chunksize
		} else {
			chunk = newJobChunk(id, done, upper, reqMsg.Data)
			remaining = 0
		}
		jobChunkList.PushBack(chunk)
	}

	return &job {
		clientId: id,
		jobChunks: jobChunkList,
		data: reqMsg.Data,
		hashVal: math.MaxUint64,
		nonce: math.MaxUint64,
		next: jobChunkList.Front().Value.(*jobChunk),
	} 
}

func (j *job) beginNextJob() {
	j.next.inSending = true
	for e := j.jobChunks.Front(); e != nil; e = e.Next() {
		if !e.Value.(*jobChunk).inSending {
			j.next = e.Value.(*jobChunk)
			return
		}
	}
	j.next = nil
}

func (j *job) receivePartialResult(finishedChunk *jobChunk, msg *bitcoin.Message) {
	// remove the jobChunk from jobChunks list.
	for e := j.jobChunks.Front(); e != nil; e = e.Next() {
		if jc := (e.Value).(*jobChunk); jc.lower == finishedChunk.lower {
			if !jc.inSending {
				LOGF.Println("Error in jobChunks, jobChunk not in sending gets result.")
			} else {
				j.jobChunks.Remove(e)
			}
			break
		}
	}

	// update the hash and nonce of the job.
	if msg.Hash < j.hashVal {
		j.hashVal = msg.Hash
		j.nonce = msg.Nonce
	} else if msg.Hash == j.hashVal {
		if msg.Nonce < j.nonce {
			j.nonce = msg.Nonce
		}
	}
}

func (j *job) receiveUnfinishedError(chunk *jobChunk) {
	// find this chunk in the list
	for e := j.jobChunks.Front(); e != nil; e = e.Next(){
		if jc := (e.Value).(*jobChunk); jc.lower == chunk.lower {
			if !jc.inSending {
				LOGF.Println("Error in jobChunks, jobChunk not in sending gets error.")
			} else {
				jc.inSending = false
				if j.next == nil || j.next.lower > jc.lower {
					j.next = jc
				}
			}
			break
		}
	}
}

type jobChunk struct {
	clientId int
	lower uint64
	upper uint64
	data string

	// flag that this jobChunk is under process by some miner.
	inSending bool
}

func newJobChunk(id int, low uint64, upp uint64, dat string) *jobChunk{
	return &jobChunk{
		clientId: id,
		lower: low,
		upper: upp,
		data: dat,
		inSending: false,
	}
}

const (
	chunksize = 10000
	name = "log_server.txt"
	flag = os.O_RDWR | os.O_CREATE
	perm = os.FileMode(0666)
)

var LOGF *log.Logger
var file *os.File
var errF error

func main() {
	defer file.Close()

	const numArgs = 2
	if len(os.Args) != numArgs {
		fmt.Println("Usage: ./server <port>")
		return
	}

	// make log file.
	file, errF = os.OpenFile(name, flag, perm)
	if errF != nil {
		return
	}
	LOGF = log.New(file, "", log.Lshortfile|log.Lmicroseconds)

	LOGF.Println("Creating new server...")
	// create new Server.
	ser := getNewServer(os.Args[1])

	myser := newMyServer(ser)

	LOGF.Println("Begin listening to messages...")
	go listenToMessage(ser, myser.joinChan, myser.reqChan, myser.resultChan, myser.errChan)
	
	for {
		select {
		case connid := <-myser.joinChan:
			LOGF.Printf("Miner %d joined...\n", connid)
			// read the message.
			_, ok := myser.minerMap[connid]
			if ok {
				LOGF.Println("Already joined miner!")
				continue
			}
			myser.minerMap[connid] = true
			myser.idles.PushBack(connid)

			// **************** Scheduler Algorithm ***************
			// **************** Algorithm Pattern! ****************
			if myser.jobQueue.Len() > 0 {
				myser.doJob()
			}

		case request := <-myser.reqChan:
			LOGF.Printf("Client %d come... \n", request.connid)
			LOGF.Println(request.message)
			// received request from client.
			newJob := createNewJob(request.connid, request.message)
			myser.connJobMap[request.connid] = newJob
			
			// the scheduler should use miner to do this job.
			// **************** Algorithm Pattern! ******************
			myser.jobQueue.PushBack(newJob)

			// if the newJob is the only job in the queue
			if myser.jobQueue.Len() == 1 {
				myser.doJob();
			}

		case result := <-myser.resultChan:
			LOGF.Printf("Result received from miner %d.\n", result.connid)
			LOGF.Println(result.message)
			// result received.
			// add the server into idle list
			myser.idles.PushBack(result.connid)

			// get the chunk that is finished
			finishedChunk := myser.minerJobMap[result.connid]
			delete(myser.minerJobMap, result.connid)

			j, ok := myser.connJobMap[finishedChunk.clientId]
			if !ok {
				LOGF.Printf("Disregard result for lost client (%d).\n", result.connid)
				continue
			}

			// checkpoint.
			if j != myser.jobQueue.Front().Value.(*job) {
				LOGF.Println("Wrong result, scheduler logic is compromised.")
				return
			}

			// renew the job that this chunk belongs to.
			j.receivePartialResult(finishedChunk, result.message)
			// LOGF.Printf("%d jobChunks in client %d left.", j.jobChunks.Len(), j.clientId)
			if j.jobChunks.Len() == 0 {
				LOGF.Printf("The job of client %d is finished, sending back the result...\n", j.clientId)

				// send the Result back to the client.
				res := bitcoin.NewResult(j.hashVal, j.nonce)
				writeMsg(ser, j.clientId, res)
				myser.jobQueue.Remove(myser.jobQueue.Front())

				(*ser).CloseConn(j.clientId)
				delete(myser.connJobMap, j.clientId)
			}

			// tell myser to use the idle miner do new job.
			myser.doJob()

		case connid := <-myser.errChan:
			// the connection to this connid is lost.
			// the connection is a client.
			_, ok := myser.connJobMap[connid]
			if ok {
				LOGF.Printf("The client %d is lost.\n", connid)
				// delete the job in the jobQueue.
				for e := myser.jobQueue.Front(); e != nil; e = e.Next() {
					if e.Value.(*job).clientId == connid {
						myser.jobQueue.Remove(e)
						break
					} 
				}

				// delete this client from server's connidToJob map.
				delete(myser.connJobMap, connid)
				myser.doJob()
				continue
			}

			// the connection is a miner
			_, ok = myser.minerMap[connid]
			if ok {
				LOGF.Printf("The miner %d is lost.\n", connid)
				// delete this miner from the miner map.
				delete(myser.minerMap, connid)

				unfinishedChunk, ok := myser.minerJobMap[connid]
				// if the miner is idle.
				if !ok {
					// remove it from the idle list.
					for e := myser.idles.Front(); e != nil; e = e.Next() {
						if e.Value.(int) == connid {
							myser.idles.Remove(e)
							break
						}
					}
					continue
				}

				// if the miner is doing job, get the unfinished chunk. 
				// and then delete the miner from miner job map.
				delete(myser.minerJobMap, connid)

				j, ok := myser.connJobMap[unfinishedChunk.clientId]
				if !ok {
					// the client already lost
					continue
				}

				j.receiveUnfinishedError(unfinishedChunk)
				myser.doJob()
			}
		}
	}
}

func (myser *myServer) doJob() {
	LOGF.Printf("doJob called, the length of the jobQueue: %d.\n", myser.jobQueue.Len())
	if myser.jobQueue.Len() == 0 || myser.idles.Len() == 0 {
		return
	}

	j := myser.jobQueue.Front().Value.(*job)
	// distribute the job.
	for jobChu := j.next; jobChu != nil; {
		// get the idle miner.
		idleMiner := myser.idles.Front()
		if idleMiner == nil {
			break
		}
		minerId := idleMiner.Value.(int)
		LOGF.Printf("Send jobChunk[%d %s %d %d] to Miner %d.\n", jobChu.clientId, jobChu.data, jobChu.lower, jobChu.upper, minerId)
		// write the message.
		msg := bitcoin.NewRequest(jobChu.data, jobChu.lower, jobChu.upper)
		writeMsg(myser.ser, minerId, msg)
		
		// record the change.
		myser.minerJobMap[minerId] = jobChu
		myser.idles.Remove(idleMiner)
		j.beginNextJob()
		jobChu = j.next
	}
}

type reqresPacket struct {
	message *bitcoin.Message
	connid int
}

type connectLostPacket struct {
	connid int
}

func listenToMessage(ser *lsp.Server, joinChan chan<- int, reqChan chan<- *reqresPacket, resChan chan<- *reqresPacket, errChan chan<- int) {
	for {
		msg, id, err := readMsg(ser)

		if err != nil {
			errChan <- id
		} else if msg.Type == bitcoin.Join { 
			joinChan <- id
		} else if msg.Type == bitcoin.Request {
			reqChan <- &reqresPacket{message: msg, connid: id}
		} else if msg.Type == bitcoin.Result {
			resChan <- &reqresPacket{message: msg, connid: id}
		} else {
			LOGF.Printf("Message type error. The type num is %d\n", msg.Type)
		}
	}
}

func getNewServer(p string) *lsp.Server {
	port, err := strconv.Atoi(p)
	param := lsp.NewParams()
	param.WindowSize = 5

	server, err := lsp.NewServer(port, param)
	if err != nil {
		LOGF.Println("Failed to create server")
		return nil
	}
	return &server
}

func readMsg(ser *lsp.Server) (*bitcoin.Message, int, error) {
	// read the messsage.
	connid, readBytes, err := (*ser).Read()
	if err != nil {
		LOGF.Printf("Server read error from connection %d.\n", connid)
		return nil, connid, err
	}

	// Unmarshal the result.
	var result bitcoin.Message
	err = json.Unmarshal(readBytes, &result)
	if err != nil {
		LOGF.Println("Unmarshal failure.")
	}
	return &result, connid, nil
}

func writeMsg(ser *lsp.Server, connid int, msg *bitcoin.Message) error {
	// marshal the message, send the message and read the message.
	bytes, err := json.Marshal(msg)
	if err != nil {
		LOGF.Println("Marshal failure.")
		return err
	}

	// send the message.
	err = (*ser).Write(connid, bytes)
	if err != nil {
		LOGF.Println("Client failed writing.")
		return err
	}
	return nil
}