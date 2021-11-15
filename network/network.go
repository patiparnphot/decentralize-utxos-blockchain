package network

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"runtime"
	"syscall"

	"github.com/vrecan/death/v3"

	"github.com/patiparnphot/decentralize-utxos-blockchain/blockchain"
)

const (
	Protocol      = "tcp"
	version       = 1
	CommandLength = 12
)

var (
	NodeAddress     string
	mineAddress     string
	KnownNodes      = []string{"localhost:3000"}
	blocksInTransit = [][]byte{}
	memoryPool      = make(map[string]blockchain.Transaction)
)

type Addr struct {
	AddrList []string
}

type Block struct {
	AddrFrom string
	Block    []byte
}

type GetBlocks struct {
	AddrFrom string
}

type GetData struct {
	AddrFrom string
	Type     string
	ID       []byte
}

type Inv struct {
	AddrFrom string
	Type     string
	Items    [][]byte
}

type Tx struct {
	AddrFrom    string
	Transaction []byte
}

type Version struct {
	Version    int
	BestHeight int
	AddrFrom   string
}

func CmdToBytes(cmd string) []byte {
	var bytes [CommandLength]byte

	for i, c := range cmd {
		bytes[i] = byte(c)
	}

	return bytes[:]
}

func BytesToCmd(bytes []byte) string {
	var cmd []byte

	for _, b := range bytes {
		if b != 0x0 {
			cmd = append(cmd, b)
		}
	}

	return fmt.Sprintf("%s", cmd)
}

func ExtractCmd(request []byte) []byte {
	return request[:CommandLength]
}

func RequestBlocks() {
	for _, node := range KnownNodes {
		SendGetBlocks(node)
	}
}

func SendAddr(address string) {
	nodes := Addr{KnownNodes}
	nodes.AddrList = append(nodes.AddrList, NodeAddress)
	payload := GobEncode(nodes)
	request := append(CmdToBytes("addr"), payload...)

	SendData(address, request)
}

func SendBlock(addr string, b *blockchain.Block) {
	data := Block{NodeAddress, b.Serialize()}
	payload := GobEncode(data)
	request := append(CmdToBytes("block"), payload...)

	SendData(addr, request)
}

func SendGenesis(addr string, b *blockchain.Block) {
	data := Block{NodeAddress, b.Serialize()}
	payload := GobEncode(data)
	request := append(CmdToBytes("genesis"), payload...)

	SendData(addr, request)
}

func SendData(addr string, data []byte) {
	conn, err := net.Dial(Protocol, addr)

	if err != nil {
		fmt.Printf("%s is not available\n", addr)
		var updatedNodes []string

		for _, node := range KnownNodes {
			if node != addr {
				updatedNodes = append(updatedNodes, node)
			}
		}

		KnownNodes = updatedNodes

		return
	}

	defer conn.Close()

	_, err = io.Copy(conn, bytes.NewReader(data))
	if err != nil {
		log.Panic(err)
	}
}

func SendInv(address, kind string, items [][]byte) {
	inventory := Inv{NodeAddress, kind, items}
	payload := GobEncode(inventory)
	request := append(CmdToBytes("inv"), payload...)

	SendData(address, request)
}

func SendGetBlocks(address string) {
	payload := GobEncode(GetBlocks{NodeAddress})
	request := append(CmdToBytes("getblocks"), payload...)

	SendData(address, request)
}

func SendGetData(address, kind string, id []byte) {
	payload := GobEncode(GetData{NodeAddress, kind, id})
	request := append(CmdToBytes("getdata"), payload...)

	SendData(address, request)
}

func SendTx(addr string, tnx *blockchain.Transaction) {
	data := Tx{NodeAddress, tnx.Serialize()}
	payload := GobEncode(data)
	request := append(CmdToBytes("tx"), payload...)

	SendData(addr, request)
}

func SendBootnode(addr string) {
	payload := GobEncode(GetBlocks{NodeAddress})
	request := append(CmdToBytes("bootnode"), payload...)

	SendData(addr, request)
}

func SendVersion(addr string, chain *blockchain.BlockChain) {
	bestHeight := chain.GetBestHeight()
	payload := GobEncode(Version{version, bestHeight, NodeAddress})

	request := append(CmdToBytes("version"), payload...)

	SendData(addr, request)
}

func HandleAddr(request []byte) {
	var buff bytes.Buffer
	var payload Addr

	buff.Write(request[CommandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)

	}

	KnownNodes = append(KnownNodes, payload.AddrList...)
	fmt.Printf("there are %d known nodes\n", len(KnownNodes))
	RequestBlocks()
}

func HandleBlock(request []byte, chain *blockchain.BlockChain, remoteIP string) {
	var buff bytes.Buffer
	var payload Block

	buff.Write(request[CommandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	blockData := payload.Block
	block := blockchain.Deserialize(blockData)

	fmt.Println("Recevied a new block!")
	chain.AddBlock(block)

	fmt.Printf("Added block %x\n", block.Hash)

	if len(blocksInTransit) > 0 {
		blockHash := blocksInTransit[0]
		SendGetData(fmt.Sprintf("%s%s", remoteIP, payload.AddrFrom), "block", blockHash)

		blocksInTransit = blocksInTransit[1:]
	} else {
		UTXOSet := blockchain.UTXOSet{chain}
		UTXOSet.Reindex()
	}
}

func HandleGenesis(request []byte) *blockchain.BlockChain {
	var buff bytes.Buffer
	var payload Block

	nodeID := os.Getenv("NODE_ID")

	buff.Write(request[CommandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	blockData := payload.Block
	genesisBlock := blockchain.Deserialize(blockData)

	fmt.Println("Recevied genesis block!")
	chain := blockchain.InitGenesis(*genesisBlock, nodeID)

	fmt.Printf("Genesis block %x\n", genesisBlock.Hash)

	UTXOSet := blockchain.UTXOSet{chain}
	UTXOSet.Reindex()

	return chain
}

func HandleInv(request []byte, chain *blockchain.BlockChain, remoteIP string) {
	var buff bytes.Buffer
	var payload Inv

	buff.Write(request[CommandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	// fmt.Printf("AddrFrom: %s\n", payload.AddrFrom)

	fmt.Printf("Recevied inventory with %d %s\n", len(payload.Items), payload.Type)

	if payload.Type == "block" {
		blocksInTransit = payload.Items

		blockHash := payload.Items[0]
		SendGetData(fmt.Sprintf("%s%s", remoteIP, payload.AddrFrom), "block", blockHash)

		newInTransit := [][]byte{}
		for _, b := range blocksInTransit {
			if bytes.Compare(b, blockHash) != 0 {
				newInTransit = append(newInTransit, b)
			}
		}
		blocksInTransit = newInTransit
	}

	if payload.Type == "tx" {
		txID := payload.Items[0]

		if memoryPool[hex.EncodeToString(txID)].ID == nil {
			SendGetData(fmt.Sprintf("%s%s", remoteIP, payload.AddrFrom), "tx", txID)
		}
	}
}

func HandleGetBlocks(request []byte, chain *blockchain.BlockChain, remoteIP string) {
	var buff bytes.Buffer
	var payload GetBlocks

	buff.Write(request[CommandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	// fmt.Printf("AddrFrom: %s\n", payload.AddrFrom)

	blocks := chain.GetBlockHashes()
	SendInv(fmt.Sprintf("%s%s", remoteIP, payload.AddrFrom), "block", blocks)
}

func HandleGetData(request []byte, chain *blockchain.BlockChain, remoteIP string) {
	var buff bytes.Buffer
	var payload GetData

	buff.Write(request[CommandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	// fmt.Printf("AddrFrom: %s\n", payload.AddrFrom)

	if payload.Type == "block" {
		block, err := chain.GetBlock([]byte(payload.ID))
		if err != nil {
			return
		}

		SendBlock(fmt.Sprintf("%s%s", remoteIP, payload.AddrFrom), &block)
	}

	if payload.Type == "tx" {
		txID := hex.EncodeToString(payload.ID)
		tx := memoryPool[txID]

		SendTx(fmt.Sprintf("%s%s", remoteIP, payload.AddrFrom), &tx)
	}
}

func HandleTx(request []byte, chain *blockchain.BlockChain, remoteIP string) {
	var buff bytes.Buffer
	var payload Tx

	buff.Write(request[CommandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	txData := payload.Transaction
	tx := blockchain.DeserializeTransaction(txData)
	memoryPool[hex.EncodeToString(tx.ID)] = tx

	fmt.Printf("%s, %d\n", NodeAddress, len(memoryPool))

	if len(memoryPool) >= 2 && len(mineAddress) > 0 {
		MineTx(chain)
	}

	// if nodeAddress == KnownNodes[0] {
	for _, node := range KnownNodes {
		if node != NodeAddress && node != fmt.Sprintf("%s%s", remoteIP, payload.AddrFrom) {
			SendInv(node, "tx", [][]byte{tx.ID})
		}
	}
	// }
}

func MineTx(chain *blockchain.BlockChain) {
	var txs []*blockchain.Transaction

	for id := range memoryPool {
		fmt.Printf("tx: %s\n", hex.EncodeToString(memoryPool[id].ID))
		tx := memoryPool[id]
		// if chain.VerifyTransaction(&tx) {
		txs = append(txs, &tx)
		// }
	}

	UTXOSet := blockchain.UTXOSet{chain}

	if !blockchain.CheckTransactions(txs, &UTXOSet) {
		for _, tx := range txs {
			txID := hex.EncodeToString(tx.ID)
			delete(memoryPool, txID)
		}
		txs = []*blockchain.Transaction{}
	}

	if len(txs) == 0 {
		fmt.Println("All Transactions are invalid")
		return
	}

	cbTx := blockchain.CoinbaseTx(mineAddress, "")
	txs = append(txs, cbTx)

	newBlock := chain.MineBlock(txs)
	UTXOSet.Reindex()

	fmt.Println("New Block mined")

	for _, tx := range txs {
		txID := hex.EncodeToString(tx.ID)
		delete(memoryPool, txID)
	}

	fmt.Printf("%s, %d\n", NodeAddress, len(memoryPool))

	for _, node := range KnownNodes {
		if node != NodeAddress {
			SendInv(node, "block", [][]byte{newBlock.Hash})
		}
	}

	if len(memoryPool) > 0 {
		MineTx(chain)
	}
}

func HandleVersion(request []byte, chain *blockchain.BlockChain, remoteIP string) {
	var buff bytes.Buffer
	var payload Version

	buff.Write(request[CommandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	bestHeight := chain.GetBestHeight()
	otherHeight := payload.BestHeight

	fmt.Printf("AddrFrom: %s\n", fmt.Sprintf("%s%s", remoteIP, payload.AddrFrom))

	if bestHeight < otherHeight {
		SendGetBlocks(payload.AddrFrom)
	} else if bestHeight > otherHeight {
		SendVersion(payload.AddrFrom, chain)
	}

	if !NodeIsKnown(fmt.Sprintf("%s%s", remoteIP, payload.AddrFrom)) {
		KnownNodes = append(KnownNodes, fmt.Sprintf("%s%s", remoteIP, payload.AddrFrom))
	}
}

func HandleBootnode(request []byte, chain *blockchain.BlockChain, remoteIP string) {
	var buff bytes.Buffer
	var payload Version

	buff.Write(request[CommandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	fmt.Printf("AddrFrom: %s\n", fmt.Sprintf("%s%s", remoteIP, payload.AddrFrom))
	genesisBlock := chain.GetGenesisBlock()
	SendGenesis(fmt.Sprintf("%s%s", remoteIP, payload.AddrFrom), &genesisBlock)
}

func HandleConnection(conn net.Conn, chain *blockchain.BlockChain) {
	remoteIP := ""
	if addr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		remoteIP = addr.IP.String()
		fmt.Printf("remoteIP: %s\n", remoteIP)
	}
	// fmt.Printf("localAddr: %s\n", conn.LocalAddr().String())

	req, err := ioutil.ReadAll(conn)
	defer conn.Close()

	if err != nil {
		log.Panic(err)
	}
	command := BytesToCmd(req[:CommandLength])
	fmt.Printf("Received %s command\n", command)

	switch command {
	case "addr":
		HandleAddr(req)
	case "block":
		HandleBlock(req, chain, remoteIP)
	case "inv":
		HandleInv(req, chain, remoteIP)
	case "getblocks":
		HandleGetBlocks(req, chain, remoteIP)
	case "getdata":
		HandleGetData(req, chain, remoteIP)
	case "tx":
		HandleTx(req, chain, remoteIP)
	case "version":
		HandleVersion(req, chain, remoteIP)
	case "bootnode":
		HandleBootnode(req, chain, remoteIP)
	default:
		fmt.Println("Unknown command")
	}

}

func StartServer(nodeID, minerAddress, bootnode string) {
	NodeAddress = fmt.Sprintf(":%s", nodeID)
	mineAddress = minerAddress

	path := fmt.Sprintf(blockchain.DbPath, nodeID)
	var chain *blockchain.BlockChain

	ln, err := net.Listen(Protocol, NodeAddress)
	if err != nil {
		log.Panic(err)
	}
	defer ln.Close()

	if bootnode == "" {
		if blockchain.DBexists(path) {
			chain = blockchain.ResumeBlockChain(nodeID)
			fmt.Println("Resumed chain")
		} else if minerAddress != "" {
			chain = blockchain.InitBlockChain(minerAddress, nodeID)
			fmt.Printf("Created new chain with %s\n", minerAddress)
		} else {
			fmt.Println("Must have minerAddr or init blockchain b4 or set bootnode")
			runtime.Goexit()
		}
	} else if !blockchain.DBexists(path) {

		KnownNodes[0] = bootnode

		SendBootnode(bootnode)

		conn, err := ln.Accept()
		blockchain.Handle(err)

		req, err := ioutil.ReadAll(conn)
		conn.Close()
		blockchain.Handle(err)

		command := BytesToCmd(req[:CommandLength])
		fmt.Printf("Received %s command\n", command)

		chain = HandleGenesis(req)
	} else {
		fmt.Println("To set bootnode must not have blockchain")
		runtime.Goexit()
	}

	defer chain.Database.Close()
	go CloseDB(chain)

	UTXOSet := blockchain.UTXOSet{chain}
	UTXOSet.Reindex()

	if bootnode != "" {
		SendVersion(KnownNodes[0], chain)
	}

	for {
		conn, err := ln.Accept()
		blockchain.Handle(err)

		go HandleConnection(conn, chain)
	}
}

func GobEncode(data interface{}) []byte {
	var buff bytes.Buffer

	enc := gob.NewEncoder(&buff)
	err := enc.Encode(data)
	if err != nil {
		log.Panic(err)
	}

	return buff.Bytes()
}

func NodeIsKnown(addr string) bool {
	for _, node := range KnownNodes {
		if node == addr {
			return true
		}
	}

	return false
}

func CloseDB(chain *blockchain.BlockChain) {
	d := death.NewDeath(syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	d.WaitForDeathWithFunc(func() {
		defer os.Exit(1)
		defer runtime.Goexit()
		chain.Database.Close()
	})
}
