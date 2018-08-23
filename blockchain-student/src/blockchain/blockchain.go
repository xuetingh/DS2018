package blockchain

import "labrpc"
import (
	"sync"
	"time"
	"crypto/sha256"
	"fmt"
	"strings"
	"encoding/hex"
	"encoding/binary"
	"log"
)

//
// A Go object implementing a single Blockchain peer.
//
type Blockchain struct {
	mu    sync.Mutex          // Lock to protect shared access to this peer's state
	peers []*labrpc.ClientEnd // RPC end points of all peers
	me    int                 // this peer's index into peers[]

	// Your code here for Blockchain peer related information.
	chain                []Block                      // block chain
	commandCh            chan string                  // listen to broadcast
	difficulty           int                          // difficulty level
	newBlock             Block                        // new block
	addBlockCh           chan AddBlockReply           //channel for add block
	downloadBlockchainCh chan DownloadBlockchainReply // channel for download block
}

//
// the tester calls Kill() when a Blockchain instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (bc *Blockchain) Kill() {
	// Your code here, if desired.
}

//
// Function to mine first ever block for every peer's blockchain.
// The first block is genesis block.
//
func (bc *Blockchain) createGenesisBlock() {

	// Your code to mine the genesis block.
	// It should be the first block mined by all blockchain peers on starting up.
	h := sha256.New();
	h.Write([]byte("genesis block" + GENESIS_HASH + string(0)))
	hash := fmt.Sprintf("%x", h.Sum(nil))
	block := Block{hash, GENESIS_HASH, "genesis block", 0, time.Now(), 0}
	bc.chain = append(bc.chain, block)
}

//
// Tester will invoke CreateNewBlock API on a peer for mining a new block.
// Tester will provide the Block data.
//
type CreateNewBlockArgs struct {
	BlockData string
}

type CreateNewBlockReply struct {
	// TODO
}

func (bc *Blockchain) CreateNewBlock(args *CreateNewBlockArgs, reply *CreateNewBlockReply) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	block := Block{}
	nonce := 0
	index := len(bc.chain)
	prevHash := bc.chain[len(bc.chain)-1].Hash
	// flag is set true when we encounter the nonce that adding it to hash meets the difficulty level
	flag := false
	for {
		toHash := []byte(fmt.Sprintf("%d%s%s%d", index, args.BlockData, prevHash, nonce))
		h := sha256.New()
		h.Write(toHash)
		hash := fmt.Sprintf("%x", h.Sum(nil))
		flag = check(hash, bc.difficulty)
		if flag {
			block.Hash = hash
			block.Index = len(bc.chain)
			block.Timestamp = time.Now()
			block.PreviousHash = bc.chain[block.Index-1].Hash
			bc.newBlock = block
			fmt.Printf("%d creates block %d, at %s\n", bc.me, block.Index, block.Timestamp)
			break
		}
		nonce++
	}
}

//
// helper function, to check if the preceding zeros meet the difficulty requirement
//
func check(hexHash string, i int) bool {
	// transform the hex-decimal form string to binary form string
	// to preserver leading zeros, before decoding hexHash, add "1" in the front of the string
	str := fmt.Sprintf("1%s1", hexHash)
	decoded, err := hex.DecodeString(str)
	if err != nil {
		log.Fatal(err)
	}
	binaryHash := binary.BigEndian.Uint64(decoded)
	bi := fmt.Sprintf("%b", binaryHash)
	mask := strings.Repeat("0", i)
	// also add "1" before the mask
	mask = fmt.Sprintf("1%s", mask)
	return strings.HasPrefix(bi, mask)
}

//
// A blockchain peer will invoke DownloadBlockchain RPC on another blockchain
// peer to download its blockchain.
//
type DownloadBlockchainArgs struct {
	Index int // index of the server who calls DownloadBlockchain
}

type DownloadBlockchainReply struct {
	Chain     []Block
	Ok        bool
	PeerIndex int
}

func (bc *Blockchain) DownloadBlockchain(args *DownloadBlockchainArgs, reply *DownloadBlockchainReply) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	reply.Chain = bc.chain
	reply.PeerIndex = bc.me
	fmt.Printf("node %d download from node %d\n", args.Index, bc.me)

}

//
// Invoked by tester to instruct a blockchain peer to update it's blockchain
// after reconnection.
//
func (bc *Blockchain) DownloadBlockchainFromPeers() {
	args := DownloadBlockchainArgs{bc.me}
	// send download block args to all peers
	for i := range bc.peers {
		if i == bc.me {
			continue
		}
		go func(server int) {
			reply := DownloadBlockchainReply{}
			ok := bc.peers[server].Call("Blockchain.DownloadBlockchain", &args, &reply)
			reply.Ok = ok
			bc.downloadBlockchainCh <- reply
		}(i)
	}
	bc.mu.Lock()
	maxChain := bc.chain
	maxLen := len(bc.chain)
	minTimestamp := bc.chain[maxLen-1].Timestamp
	bc.mu.Unlock()
	totalCount := len(bc.peers) - 1
	for reply := range bc.downloadBlockchainCh {
		totalCount--
		currentChain := reply.Chain
		currentLen := len(currentChain)
		if reply.Ok == false || currentLen == 0 {
			continue
		}
		currentTime := currentChain[currentLen-1].Timestamp
		if currentLen > maxLen {
			maxLen = currentLen
			maxChain = currentChain
			minTimestamp = currentTime
		} else if currentLen == maxLen && currentTime.Before(minTimestamp) {
			maxChain = currentChain
			minTimestamp = currentTime
		}
		// if collected all peer's replies
		if 0 == totalCount {
			bc.mu.Lock()
			bc.chain = maxChain
			bc.mu.Unlock()
			bc.commandCh <- "Done"
			return
		}
	}
}

//
// Invoked by tester to instruct a blockchain peer to broadcast its pending created
// block to other peers. Invoked after mining new block on this peer.
// Unless all other peers agree on this pending block,
// it cant be added to the final blockchain of this peer.
//
func (bc *Blockchain) BroadcastNewBlock() {
	bc.mu.Lock()
	args := AddBlockArgs{
		Index:        bc.newBlock.Index,
		Hash:         bc.newBlock.Hash,
		PreviousHash: bc.newBlock.PreviousHash,
		Timestamp:    bc.newBlock.Timestamp,
		Data:         bc.newBlock.Data,
		Nonce:        bc.newBlock.Nonce,
	}
	bc.mu.Unlock()

	// send add block args to all peers
	for i := range bc.peers {
		if i == bc.me {
			continue
		}
		go func(server int) {
			reply := AddBlockReply{}
			ok := bc.peers[server].Call("Blockchain.AddBlock", &args, &reply)
			reply.Ok = ok
			bc.addBlockCh <- reply
		}(i)
	}

	// get response from all peers and count
	totalCount := len(bc.peers) - 1
	for resp := range bc.addBlockCh {
		if resp.Ok && resp.Approved == false {
			// no peer died, but one disapproved, this server should mine a new block
			fmt.Printf("node %d did not get node %d's approval on block %d\n", bc.me, resp.PeerIndex, bc.newBlock.Index)
			bc.newBlock = Block{}
			go bc.DownloadBlockchainFromPeers()
			return
		} else {
			// pear died or approved, count as approved
			totalCount--
			if 0 == totalCount {
				fmt.Printf("node %d received all peer's approval on block %d\n", bc.me, bc.newBlock.Index)
				bc.mu.Lock()
				if bc.newBlock.Index == len(bc.chain) {
					// the server appends its newly mined block
					bc.chain = append(bc.chain, bc.newBlock)
					bc.newBlock = Block{}
					bc.mu.Unlock()
					bc.commandCh <- "Done"
				} else {
					// the server get its peer's block
					bc.newBlock = Block{}
					bc.mu.Unlock()
					go bc.DownloadBlockchainFromPeers()
				}
				return
			}
		}
	}
}

//
// A blockchain peer will invoke AddBlock RPC on another blockchain peer
// to request for adding its newly mined block.
//
type AddBlockArgs struct {
	Index        int       // index of this block
	Hash         string    // hash of this block
	PreviousHash string    // previous hash
	Data         string    // block data
	Timestamp    time.Time // timestamp for this block
	Nonce        int       // nonce
}

type AddBlockReply struct {
	Approved  bool
	Ok        bool
	PeerIndex int
}

func (bc *Blockchain) AddBlock(args *AddBlockArgs, reply *AddBlockReply) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	length := len(bc.chain)
	reply.PeerIndex = bc.me
	// check index, previous hash and difficulty
	if args.Index == length && args.PreviousHash == bc.chain[length-1].Hash && check(args.Hash, bc.difficulty) {
		newBlock := Block{args.Hash, args.PreviousHash, args.Data, args.Index, args.Timestamp, args.Nonce}
		bc.chain = append(bc.chain, newBlock)
		reply.Approved = true
	} else {
		reply.Approved = false
	}
}

//
// Invoked by tester to get blockchain length and last block's hash.
//
func (bc *Blockchain) GetState() (int, string) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	var blockchainSize int
	var lastBlockHash string
	blockchainSize = len(bc.chain)
	lastBlockHash = bc.chain[blockchainSize-1].Hash
	return blockchainSize, lastBlockHash
}

//
// the service or tester wants to create a Blockchain server. the ports
// of all the Blockchain servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
// Command channel is used by tester to instruct Broadcast and DownloadBlockchain
// commands to this blockchain peer.
//
func Make(peers []*labrpc.ClientEnd, me int, difficulty int, command chan string) *Blockchain {

	bc := &Blockchain{}
	bc.peers = peers
	bc.me = me

	// Your initialization code here
	bc.difficulty = difficulty
	bc.commandCh = command
	bc.chain = [] Block{}
	bc.mu.Lock()
	bc.createGenesisBlock()
	bc.mu.Unlock()
	bc.addBlockCh = make(chan AddBlockReply)
	bc.downloadBlockchainCh = make(chan DownloadBlockchainReply)
	go bc.Run()
	return bc
}

func (bc *Blockchain) Run() {
	for cmd := range bc.commandCh {
		if cmd == "Broadcast" {
			if len(bc.newBlock.Hash) > 0 {
				go bc.BroadcastNewBlock()
			}
		} else if cmd == "DownloadBlockchain" {
			go bc.DownloadBlockchainFromPeers()
		}
	}
}
