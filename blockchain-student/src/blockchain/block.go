package blockchain

import "time"

const GENESIS_HASH = "816534932c2b7154836da6afc367695e6337db8a921823784c14378abed4f7d7"

type Block struct {
	// Your code here for what goes into a blockchain's block.
	Hash         string   // hash value for this block
	PreviousHash string   // hash value for all previous block
	Data         string    // data of this block
	Index        int       // index of this block
	Timestamp    time.Time // timestamp of the block
	Nonce        int       // nouce
}
