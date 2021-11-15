package cli

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"runtime"
	"strconv"

	"github.com/patiparnphot/decentralize-utxos-blockchain/blockchain"
	"github.com/patiparnphot/decentralize-utxos-blockchain/network"
)

type CommandLine struct{}

func (cli *CommandLine) printUsage() {
	fmt.Println("Usage:")
	fmt.Println(" getbalance -address ADDRESS - get the balance for an address")
	fmt.Println(" createblockchain -address ADDRESS creates a blockchain and sends genesis reward to address")
	fmt.Println(" print - Prints the blocks in the chain")
	fmt.Println(" send -from FROM -to TO -amount AMOUNT - Send amount of coins")
	fmt.Println(" send -from FROM -to TO -amount AMOUNT -mine - Send amount of coins. Then -mine flag is set, mine off of this node")
	fmt.Println(" send -from FROM -to TO -amount AMOUNT -mine -bootnode BOOTNODE - Send amount of coins. Then -mine flag is set, mine off of this node. Then -bootnode flag is set to connect with BOOTNODE.")
	fmt.Println(" reindexutxo - Rebuilds the UTXO set")
	fmt.Println(" startnode -miner ADDRESS - Start a node with ID specified in NODE_ID env. var. -miner enables mining")
	fmt.Println(" startnode -miner ADDRESS -bootnode BOOTNODE - Start a node with ID specified in NODE_ID env. var. -miner enables mining. Then -bootnode flag is set to connect with BOOTNODE.")
}

func (cli *CommandLine) validateArgs() {
	if len(os.Args) < 2 {
		cli.printUsage()
		runtime.Goexit()
	}
}

func (cli *CommandLine) StartNode(nodeID, minerAddress, bootnode string) {
	fmt.Printf("Starting Node %s\n", nodeID)

	network.StartServer(nodeID, minerAddress, bootnode)
}

func (cli *CommandLine) reindexUTXO(nodeId string) {
	chain := blockchain.ResumeBlockChain(nodeId)
	defer chain.Database.Close()
	UTXOSet := blockchain.UTXOSet{chain}
	UTXOSet.Reindex()

	count := UTXOSet.CountTransactions()
	fmt.Printf("Done! There are %d transactions in the UTXO set.\n", count)
}

func (cli *CommandLine) printChain(nodeId string) {
	chain := blockchain.ResumeBlockChain(nodeId)
	defer chain.Database.Close()

	iter := chain.Iterator()

	for {
		block := iter.Next()

		fmt.Printf("Previous hash: %x\n", block.PrevHash)
		fmt.Printf("Hash: %x\n", block.Hash)
		fmt.Printf("Nonce: %d\n", block.Nonce)

		pow := blockchain.NewProof(block)

		fmt.Printf("PoW: %s\n", strconv.FormatBool(pow.Validate()))
		for _, tx := range block.Transactions {
			fmt.Printf("Transaction Inputs: %v\n", tx.Inputs)
			fmt.Printf("Transaction Outputs: %v\n", tx.Outputs)
		}
		fmt.Println()

		if len(block.PrevHash) == 0 {
			break
		}
	}
}

func (cli *CommandLine) createBlockchain(address, nodeId string) {
	chain := blockchain.InitBlockChain(address, nodeId)
	defer chain.Database.Close()

	UTXOSet := blockchain.UTXOSet{chain}
	UTXOSet.Reindex()

	fmt.Println("Finished!!!")
}

func (cli *CommandLine) getBalance(address, nodeId string) {
	chain := blockchain.ResumeBlockChain(nodeId)
	UTXOSet := blockchain.UTXOSet{chain}
	defer chain.Database.Close()

	balance := 0
	UTXOs := UTXOSet.FindUTXO(address)

	for _, out := range UTXOs {
		balance += out.Value
	}

	fmt.Printf("Balance of %s: %d\n", address, balance)
}

func (cli *CommandLine) send(from, to string, amount int, nodeId string, mineNow bool, bootnode string) {
	path := fmt.Sprintf(blockchain.DbPath, nodeId)
	var chain *blockchain.BlockChain

	if blockchain.DBexists(path) {
		chain = blockchain.ResumeBlockChain(nodeId)
		fmt.Println("Resumed chain")

		defer chain.Database.Close()
		UTXOSet := blockchain.UTXOSet{chain}
		UTXOSet.Reindex()

		tx := blockchain.NewTransaction(from, to, amount, &UTXOSet)
		if mineNow {
			cbTx := blockchain.CoinbaseTx(from, "")
			txs := []*blockchain.Transaction{cbTx, tx}
			block := chain.MineBlock(txs)
			UTXOSet.Update(block)
			fmt.Println("Transfer & Mine Success!!!")
		} else if bootnode != "" {
			network.KnownNodes[0] = bootnode
			network.SendTx(network.KnownNodes[0], tx)
			fmt.Println("send tx")
		} else {
			fmt.Println("Please enter bootnode or mine")
		}
	} else if bootnode == "" {
		chain = blockchain.InitBlockChain(from, nodeId)
		fmt.Printf("Created new chain with %s\n", from)

		defer chain.Database.Close()
		UTXOSet := blockchain.UTXOSet{chain}
		UTXOSet.Reindex()

		fmt.Println("Please enter the command again")
	} else {
		network.NodeAddress = fmt.Sprintf(":%s", nodeId)

		ln, err := net.Listen(network.Protocol, network.NodeAddress)

		network.KnownNodes[0] = bootnode
		network.SendBootnode(bootnode)

		conn, err := ln.Accept()
		blockchain.Handle(err)

		req, err := ioutil.ReadAll(conn)
		conn.Close()
		blockchain.Handle(err)

		command := network.BytesToCmd(req[:network.CommandLength])
		fmt.Printf("Received %s command\n", command)

		chain = network.HandleGenesis(req)
		defer chain.Database.Close()

		network.SendVersion(bootnode, chain)
		fmt.Println("Please enter the command again")
	}
}

func (cli *CommandLine) Run() {
	cli.validateArgs()

	nodeID := os.Getenv("NODE_ID")
	if nodeID == "" {
		fmt.Printf("NODE_ID env is not set!")
		runtime.Goexit()
	}

	getBalanceCmd := flag.NewFlagSet("getbalance", flag.ExitOnError)
	createBlockchainCmd := flag.NewFlagSet("createblockchain", flag.ExitOnError)
	sendCmd := flag.NewFlagSet("send", flag.ExitOnError)
	printChainCmd := flag.NewFlagSet("print", flag.ExitOnError)
	reindexUTXOCmd := flag.NewFlagSet("reindexutxo", flag.ExitOnError)
	startNodeCmd := flag.NewFlagSet("startnode", flag.ExitOnError)

	getBalanceAddress := getBalanceCmd.String("address", "", "the address to get balance for")
	createBlockchainAddress := createBlockchainCmd.String("address", "", "the address to send genesis reward to")
	sendFrom := sendCmd.String("from", "", "sender address")
	sendTo := sendCmd.String("to", "", "receiver address")
	sendAmount := sendCmd.Int("amount", 0, "amount to send")
	sendMine := sendCmd.Bool("mine", false, "Mine immediately on the same node")
	sendBootnode := sendCmd.String("bootnode", "", "Enable bootnode mode")
	startNodeMiner := startNodeCmd.String("miner", "", "Enable mining mode and send reward to ADDRESS")
	startNodeBootnode := startNodeCmd.String("bootnode", "", "Enable bootnode mode")

	switch os.Args[1] {
	case "startnode":
		err := startNodeCmd.Parse(os.Args[2:])
		blockchain.Handle(err)

	case "getbalance":
		err := getBalanceCmd.Parse(os.Args[2:])
		blockchain.Handle(err)

	case "createblockchain":
		err := createBlockchainCmd.Parse(os.Args[2:])
		blockchain.Handle(err)

	case "send":
		err := sendCmd.Parse(os.Args[2:])
		blockchain.Handle(err)

	case "print":
		err := printChainCmd.Parse(os.Args[2:])
		blockchain.Handle(err)

	case "reindexutxo":
		err := reindexUTXOCmd.Parse(os.Args[2:])
		blockchain.Handle(err)

	default:
		cli.printUsage()
		runtime.Goexit()
	}

	if getBalanceCmd.Parsed() {
		if *getBalanceAddress == "" {
			getBalanceCmd.Usage()
			runtime.Goexit()
		} else {
			cli.getBalance(*getBalanceAddress, nodeID)
		}
	}

	if createBlockchainCmd.Parsed() {
		if *createBlockchainAddress == "" {
			createBlockchainCmd.Usage()
			runtime.Goexit()
		} else {
			cli.createBlockchain(*createBlockchainAddress, nodeID)
		}
	}

	if sendCmd.Parsed() {
		if *sendFrom == "" || *sendTo == "" || *sendAmount <= 0 {
			sendCmd.Usage()
			runtime.Goexit()
		} else {
			cli.send(*sendFrom, *sendTo, *sendAmount, nodeID, *sendMine, *sendBootnode)
		}
	}

	if printChainCmd.Parsed() {
		cli.printChain(nodeID)
	}

	if reindexUTXOCmd.Parsed() {
		cli.reindexUTXO(nodeID)
	}

	if startNodeCmd.Parsed() {
		if nodeID == "" {
			startNodeCmd.Usage()
			runtime.Goexit()
		}
		cli.StartNode(nodeID, *startNodeMiner, *startNodeBootnode)
	}
}
