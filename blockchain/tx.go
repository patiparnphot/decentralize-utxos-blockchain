package blockchain

import (
	"bytes"
	"encoding/gob"
)

type TxInput struct {
	ID  []byte
	Out int
	Sig string
}

type TxOutput struct {
	Value  int
	PubKey string
}

type TxOutputs struct {
	Outputs []TxOutput
}

func (input *TxInput) CanUnlock(data string) bool {
	return input.Sig == data
}

func (output *TxOutput) CanBeUnlocked(data string) bool {
	return output.PubKey == data
}

func (outs TxOutputs) Serialize() []byte {
	var buffer bytes.Buffer

	encode := gob.NewEncoder(&buffer)
	err := encode.Encode(outs)
	Handle(err)

	return buffer.Bytes()
}

func DeserializeOutputs(data []byte) TxOutputs {
	var outputs TxOutputs

	decode := gob.NewDecoder(bytes.NewReader(data))
	err := decode.Decode(&outputs)
	Handle(err)

	return outputs
}
