package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//
import (
	"labrpc"
	"math/rand"
	"sync"
	"time"
	"fmt"
)

const BUFFER = 100
const ELECTIME = 400 * time.Millisecond
const HeartBeatTIME   = 120 * time.Millisecond

// role of server
const (
	LEADER = iota
	CANDIDATE
	FOLLOWER
)


//
// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make().
//
type ApplyMsg struct {
	Index       int
	Command     interface{}
	UseSnapshot bool   // ignore for lab2; only used in lab3
	Snapshot    []byte // ignore for lab2; only used in lab3
}

//
// A Go object of log entry
//

type LogEntry struct {
	Command interface{}
	Index   int
	Term    int
}

//
// A Go object implementing a single Raft peer.
//
type Raft struct {
	mu    sync.Mutex          // Lock to protect shared access to this peer's state
	peers []*labrpc.ClientEnd // RPC end points of all peers
	me    int                 // this peer's index into peers[]

	state     int
	voteCount int
    // persistent state on all servers, update on stable storage before responding to RPCs
	currentTerm int
	votedFor    int
	log         []LogEntry
    // state on all servers
	commitIndex int
	lastApplied int
    // state on leaders, reinitialize after election
	nextIndex  []int
	matchIndex []int
    //
	grantVoteCh  chan bool
	appendEntryCh chan bool
	majorityVoteCh    chan bool
	commitCh     chan bool
	applyCh      chan ApplyMsg
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	return rf.currentTerm, rf.state == LEADER
}

// return last index from its log
func (rf *Raft) getLastIndex() int {
	return rf.log[len(rf.log) - 1].Index
}

// return last term from its log
func (rf *Raft) getLastTerm() int {
	return rf.log[len(rf.log) - 1].Term
}

// example RequestVote RPC arguments structure.
// field names must start with capital letters!
//
type RequestVoteArgs struct {
	Term         int            // candidate's term
	CandidateId  int            // candidate requesting vote
	LastLogIndex int            // index of candidates's last log entry
	LastLogTerm  int            // term of candidate's last log entry
}

//
// example RequestVote RPC reply structure.
// field names must start with capital letters!
//
type RequestVoteReply struct {
	Term        int             // current term, for candidate to update itself
	VoteGranted bool            // true means candidate received vote
}

type AppendEntriesArgs struct {
	Term         int            // leader's term
	LeaderId     int            // leader's id
	PrevLogIndex int            // index of log entry immediately preceding new ones
	PrevLogTerm  int            // term of prevLogIndex entry
	Entries      []LogEntry     // log entries to store
	LeaderCommit int            // leader's commit index
}

type AppendEntriesReply struct {
	Term    int                 // current term, for leader to update itself
	Success bool                // true if follower contained entry matching
	                            // prevLogIndex and prevLogTerm
	NextIndex int               // tell the leader where the append should start
}


//
// example RequestVote RPC handler.
//
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

    reply.Term = rf.currentTerm
    reply.VoteGranted = false

	// if the requester's term is behind this server, return false
	if args.Term < rf.currentTerm {
		return
	}
	if args.Term > rf.currentTerm {
		//if the requester's term is ahead of this server, this server should be a follower
		rf.currentTerm = args.Term
		rf.state = FOLLOWER
		rf.votedFor = -1
	}
    // If votedFor is null or candidateID and candidate's log is up-to-date, grant vote
	if (rf.votedFor == -1 || rf.votedFor == args.CandidateId) && rf.CompareLog(args.LastLogIndex, args.LastLogTerm) {
		rf.state = FOLLOWER
		rf.votedFor = args.CandidateId
		rf.grantVoteCh <- true
		reply.VoteGranted = true
	}
	return
}

//
//  Helper function: if candidate's log is at least as up-to-date as this server, return true
//
func (rf *Raft) CompareLog(index, term int) bool {
    myIndex := rf.getLastIndex()
    myTerm := rf.getLastTerm()
    if myTerm < term || (myTerm == term && myIndex <= index) {
        return true
    } else {
        return false
    }
}

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	reply.Success = false
    // reply false if term is behind this server's current term
	if args.Term < rf.currentTerm {
		reply.Term = rf.currentTerm
        reply.NextIndex = rf.getLastIndex() + 1
		return
	}
	rf.appendEntryCh <- true
	if args.Term > rf.currentTerm {
		rf.currentTerm = args.Term
		rf.state = FOLLOWER
		rf.votedFor = -1
	}
	reply.Term = args.Term
	if args.PrevLogIndex > rf.getLastIndex() {
    		reply.NextIndex = rf.getLastIndex() + 1
    		return
    }

    if args.PrevLogIndex > 0 {
    	term := rf.log[args.PrevLogIndex].Term
    	if args.PrevLogTerm != term {
   			for i := args.PrevLogIndex - 1 ; i >= 0; i-- {
 				if rf.log[i].Term != term {
    				reply.NextIndex = i + 1
    				break
    			}
    		}
    		return
  		}
	    reply.NextIndex = rf.getLastIndex() + 1
    }
    rf.log = rf.log[:args.PrevLogIndex + 1]
    rf.log = append(rf.log, args.Entries...)
	if args.LeaderCommit > rf.commitIndex {
		// set commitIndex to min(leaderCommit, index of last new entry)
		if args.LeaderCommit < rf.getLastIndex() {
		    rf.commitIndex = args.LeaderCommit
		} else {
		    rf.commitIndex = rf.getLastIndex()
		}
		rf.commitCh <- true
	}
	reply.Term = rf.currentTerm
	reply.Success = true
	return
}

//
// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
//
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if ok {
		if reply.Term > rf.currentTerm {
			rf.currentTerm = reply.Term
			rf.state = FOLLOWER
			rf.votedFor = -1
		} else if reply.VoteGranted {
			rf.voteCount++
			majority := len(rf.peers) / 2
			if rf.state == CANDIDATE && rf.voteCount > majority {
				rf.majorityVoteCh <- true
			}
		}
	}
	return ok
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if ok {
		if reply.Term > rf.currentTerm {
			rf.currentTerm = reply.Term
			rf.state = FOLLOWER
			rf.votedFor = -1
			return ok
		}
		if reply.Success {
	        if len(args.Entries) > 0 {
                rf.nextIndex[server] = args.Entries[len(args.Entries) - 1].Index + 1
       			rf.matchIndex[server] = rf.nextIndex[server] - 1
            }
        } else {
            rf.nextIndex[server] = reply.NextIndex
        }
	}
	return ok
}

//
// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
//
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	index := -1
	term := rf.currentTerm
	isLeader := false
	if rf.state == LEADER {
		isLeader = true
		index = rf.getLastIndex() + 1
		log := LogEntry{
			Term:    term,
			Command: command,
			Index:   index,
		}
		rf.log = append(rf.log, log)
	}
	return index, term, isLeader
}

//
// the tester calls Kill() when a Raft instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (rf *Raft) Kill() {
	// Your code here, if desired.
}

//
// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
//
func Make(peers []*labrpc.ClientEnd, me int, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.me = me
	rf.state = FOLLOWER
	rf.votedFor = -1
	rf.log = append(rf.log, LogEntry{Index: 0, Term: 0})
	rf.currentTerm = 0
	rf.nextIndex = make([]int, len(rf.peers))
	rf.matchIndex = make([]int, len(rf.peers))

	rf.appendEntryCh = make(chan bool, BUFFER)
	rf.grantVoteCh = make(chan bool, BUFFER)
	rf.majorityVoteCh = make(chan bool, BUFFER)
	rf.commitCh = make(chan bool, BUFFER)
	rf.applyCh = applyCh
	go rf.run()
	go rf.applyMessage()
	return rf
}

// start and run a raft server, different roles behaves differently
func (rf *Raft) run() {
	for {
		rf.mu.Lock()
		switch rf.state {
		case FOLLOWER:
			rf.mu.Unlock()
			ticker := time.After(time.Duration(rand.Int63n(100))*time.Millisecond + ELECTIME)
			select {
			case <-ticker:
				rf.mu.Lock()
				rf.state = CANDIDATE
				rf.mu.Unlock()
			case <-rf.grantVoteCh:
			case <-rf.appendEntryCh:
			}
		case CANDIDATE:
			rf.currentTerm++
			rf.votedFor = rf.me
			rf.voteCount = 1
			ticker := time.After(time.Duration(rand.Int63n(100))*time.Millisecond + ELECTIME)
			//sends out Request Vote to all rafts
			args := &RequestVoteArgs{
				Term:         rf.currentTerm,
				CandidateId:  rf.me,
				LastLogTerm:  rf.getLastTerm(),
				LastLogIndex: rf.getLastIndex(),
			}
			rf.mu.Unlock()
			for server := range rf.peers {
				if server != rf.me {
					var reply RequestVoteReply
					go rf.sendRequestVote(server, args, &reply)
				}
			}
			select {
			case <-ticker:
			case <-rf.appendEntryCh: 
				rf.mu.Lock()
				rf.state = FOLLOWER
				rf.votedFor = -1
				rf.mu.Unlock()
			case <-rf.majorityVoteCh:
				rf.mu.Lock()
				rf.state = LEADER
				for i := range rf.peers {
					rf.nextIndex[i] = rf.getLastIndex() + 1
					rf.matchIndex[i] = 0
				}
				rf.mu.Unlock()
			}
		case LEADER:
			for i := rf.commitIndex + 1; i <= rf.getLastIndex(); i++ {
				majority := len(rf.peers) / 2
				for j := range rf.peers {
					if rf.matchIndex[j] >= i && rf.log[i].Term == rf.currentTerm {
						majority--
						if majority <= 0 {
							rf.commitIndex = i
							rf.commitCh <- true
							break
						}
					}
				}
			}
			for server := range rf.peers {
				if server != rf.me {
					args := &AppendEntriesArgs{
						Term:         rf.currentTerm,
						LeaderId:     rf.me,
						LeaderCommit: rf.commitIndex,
						PrevLogIndex: rf.nextIndex[server] - 1,
					}
					if args.PrevLogIndex >= 0 && args.PrevLogIndex < len(rf.log) {
						args.PrevLogTerm = rf.log[args.PrevLogIndex].Term
						args.Entries = make([]LogEntry, len(rf.log[args.PrevLogIndex + 1:]))
						copy(args.Entries, rf.log[args.PrevLogIndex + 1:])
					}
					//fmt.Println(args.PrevLogIndex)
					//fmt.Println(rf.log[args.PrevLogIndex].Command)
					var reply AppendEntriesReply
					go rf.sendAppendEntries(server, args, &reply)
				}
			}
			rf.mu.Unlock()
			time.Sleep(HeartBeatTIME)
		}
	}
}

// each time a new entry is committed to the log, each Raft peer
// should send an |ApplyMsg| to the tester via the |applyCh| passed to type

func (rf *Raft) applyMessage() {
	for {
		select {
		case <-rf.commitCh:
			rf.mu.Lock()
			for i := rf.lastApplied + 1; i <= rf.commitIndex; i++ {
				args := ApplyMsg{
					Index:   i,
					Command: rf.log[i].Command,
				}
				//fmt.Println(rf.log[i].Command)
				rf.applyCh <- args
				fmt.Println("server ", rf.me, "committed", args.Command, "Term:", rf.currentTerm, "Index:", args.Index)
			}
			rf.lastApplied = rf.commitIndex
			rf.mu.Unlock()
		}
	}
}
