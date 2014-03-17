package main

import (
	"fmt"
	"os"
	"github.com/cmu440/lsp"
	// "github.com/cmu440/lspnet"
	"log"
	"encoding/json"
	"strconv"
	// "errors"
	"github.com/cmu440/bitcoin"
)

const (
	name = "log.txt"
	flag = os.O_RDWR | os.O_CREATE
	perm = os.FileMode(0666)
)

var LOGF *log.Logger

func main() {
	file, errF := os.OpenFile(name, flag, perm)
	if errF != nil {
		return
	}
	LOGF = log.New(file, "", log.Lshortfile|log.Lmicroseconds)

	const numArgs = 4
	if len(os.Args) != numArgs {
		fmt.Println("Usage: ./client <hostport> <message> <maxNonce>")
		return
	}

	LOGF.Println("Creating new client...")
	// create new Client.
	cli := getNewClient(os.Args[1])

	LOGF.Println("Sending request...")
	// create the message.
	reqMsg, _ := getNewRequest(os.Args)

	err := writeMsg(cli, reqMsg)
	if err != nil {
		printDisconnected()
	}

	LOGF.Println("Reading response...")
	res, errR := readMsg(cli)
	if errR != nil {
		LOGF.Println("Reading error, quit")
		printDisconnected()
		return
	}

	if res.Type != bitcoin.Result {
		LOGF.Println("Not a result.")
		return
	}
	// Print.
	LOGF.Println("Successfully received the result, printing result.")
	printResult(strconv.FormatUint(res.Hash, 10), strconv.FormatUint(res.Nonce, 10))
	file.Close()
}

// printResult prints the final result to stdout.
func printResult(hash, nonce string) {
	fmt.Println("Result", hash, nonce)
}

// printDisconnected prints a disconnected message to stdout.
func printDisconnected() {
	fmt.Println("Disconnected")
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

func getNewRequest(args []string) (*bitcoin.Message, error) {
	data := os.Args[2]
	var lowerBound uint64
	lowerBound = 0
	upperBound, err := strconv.ParseUint(os.Args[3], 10, 64)

	if err != nil {
		LOGF.Println("Wrong input arguments")
	}

	reqMsg := bitcoin.NewRequest(data, lowerBound, upperBound)
	return reqMsg, nil
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