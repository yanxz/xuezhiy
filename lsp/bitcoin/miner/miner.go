package main

import (
	"fmt"
	"os"
	"github.com/cmu440/lsp"
	//"lspnet"
	"log"
	"encoding/json"
	//"strconv"
	"math"
	"github.com/cmu440/bitcoin"
)

const (
	name = "log_miner.txt"
	flag = os.O_RDWR | os.O_CREATE
	perm = os.FileMode(0666)
)

var LOGF *log.Logger
var file *os.File
var errF error

func main() {
	defer file.Close()

	file, errF = os.OpenFile(name, flag, perm)
	if errF != nil {
		return
	}
	LOGF = log.New(file, "", log.Lshortfile|log.Lmicroseconds)

	const numArgs = 2
	if len(os.Args) != numArgs {
		fmt.Println("Usage: ./miner <hostport>")
		return
	}

	LOGF.Println("Creating new miner...")
	// create new Client.
	miner := getNewClient(os.Args[1])

	LOGF.Println("Join the server...")
	// create new Join message.
	joinMsg := bitcoin.NewJoin()

	// marshal the message, send the message and read the message.
	err := writeMsg(miner, joinMsg)
	if err != nil {
		return
	}
	
	LOGF.Println("Successfully joined the server, now waiting for jobs...")
	for {
		// read the job message from the server.
		job, errR := readMsg(miner)
		if errR != nil {
			LOGF.Println("Reading error, quit")
			return;
		}

		if job.Type != bitcoin.Request {
			LOGF.Println("Not a request.")
			continue
		}
		partialResultMsg := handleJob(job)

		err = writeMsg(miner, partialResultMsg)
		if err != nil {
			return
		}
	}
}

func handleJob(job *bitcoin.Message) *bitcoin.Message {
	low := job.Lower
	high := job.Upper
	msg := job.Data

	var min, nonce uint64
	min = math.MaxUint64

	for i := low; i <= high; i++ {
		if val := bitcoin.Hash(msg, i); val < min {
			nonce = i
			min = val
		}
	}

	return bitcoin.NewResult(min, nonce)
}

func getNewClient(hostport string) *lsp.Client {
	// client parameter: EpochLimit: 5, EpochMillis: 2000, WindowSize: 5.
	param := lsp.NewParams()
	param.WindowSize = 5
	
	cli, err := lsp.NewClient(hostport, param)
	if err != nil {
		LOGF.Println("Failed to create new client.")
	}
	return &cli
}

func writeMsg(cli *lsp.Client, msg *bitcoin.Message) error {
	// marshal the message, send the message and read the message.
	bytes, err := json.Marshal(msg)
	if err != nil {
		LOGF.Println("Marshal failure.")
		return err
	}

	// send the message.
	err = (*cli).Write(bytes)
	if err != nil {
		LOGF.Println("Client failed writing.")
		return err
	}
	return nil
}

func readMsg(cli *lsp.Client) (*bitcoin.Message, error) {
	// read the messsage.
	readBytes, err := (*cli).Read()
	if err != nil {
		LOGF.Println("Client failed reading.")
		return nil, err
	}

	// Unmarshal the result.
	var result bitcoin.Message
	err = json.Unmarshal(readBytes, &result)
	if err != nil {
		LOGF.Println("Unmarshal failure.")
	}
	return &result, nil
}