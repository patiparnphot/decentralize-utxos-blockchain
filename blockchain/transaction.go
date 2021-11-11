package blockchain

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"log"
)

type Transaction struct {
	ID      []byte
	Inputs  []TxInput
	Outputs []TxOutput
}

func (tx *Transaction) Hash() []byte {
	var hash [32]byte

	txCopy := *tx
	txCopy.ID = []byte{}

	hash = sha256.Sum256(txCopy.Serialize())

	return hash[:]
}

func (tx Transaction) Serialize() []byte {
	var encoded bytes.Buffer

	enc := gob.NewEncoder(&encoded)
	err := enc.Encode(tx)
	if err != nil {
		log.Panic(err)
	}

	return encoded.Bytes()
}

func DeserializeTransaction(data []byte) Transaction {
	var transaction Transaction

	decoder := gob.NewDecoder(bytes.NewReader(data))
	err := decoder.Decode(&transaction)
	Handle(err)
	return transaction
}

func CoinbaseTx(to, data string) *Transaction {
	if data == "" {
		randData := make([]byte, 20)
		_, err := rand.Read(randData)
		Handle(err)

		data = fmt.Sprintf("%x", randData)
	}

	txin := TxInput{[]byte{}, -1, data}
	txout := TxOutput{33, to}

	tx := Transaction{nil, []TxInput{txin}, []TxOutput{txout}}
	tx.ID = tx.Hash()

	return &tx
}

func NewTransaction(from, to string, amount int, UTXO *UTXOSet) *Transaction {
	var inputs []TxInput
	var outputs []TxOutput

	acc, validOutputs := UTXO.FindSpendableOutputs(from, amount)

	if acc < amount {
		log.Panic("Error: Not enough funds")
	}

	for txid, outs := range validOutputs {
		txID, err := hex.DecodeString(txid)
		Handle(err)

		for _, out := range outs {
			input := TxInput{txID, out, from}
			inputs = append(inputs, input)
		}
	}

	outputs = append(outputs, TxOutput{amount, to})

	if acc > amount {
		outputs = append(outputs, TxOutput{acc - amount, from})
	}

	tx := Transaction{nil, inputs, outputs}
	tx.ID = tx.Hash()

	return &tx
}

func CheckTransactions(transactions []*Transaction, UTXO *UTXOSet) bool {
	amountMap := map[string]int{}

	for _, transaction := range transactions {

		inputs := transaction.Inputs
		outputs := transaction.Outputs

		from := inputs[0].Sig
		var amount int

		for _, out := range outputs {
			if out.PubKey != from {
				amount = out.Value
			}
		}

		found := false

		for addr, _ := range amountMap {
			if addr == from {
				found = true
				amountMap[from] += amount
			}
		}

		if !found {
			amountMap[from] = amount
		}
	}

	for addr, totalAmount := range amountMap {
		acc, _ := UTXO.FindSpendableOutputs(addr, totalAmount)
		fmt.Printf("accumulate is %d, totalAmount is %d\n", acc, totalAmount)

		if acc < totalAmount {
			fmt.Printf("%s have not enough funds\n", addr)
			return false
		}
	}

	return true
}

func (tx *Transaction) IsCoinbase() bool {
	return len(tx.Inputs) == 1 && len(tx.Inputs[0].ID) == 0 && tx.Inputs[0].Out == -1
}
