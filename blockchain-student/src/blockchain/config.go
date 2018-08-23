package blockchain

import "labrpc"
import "sync"
import "testing"
import "runtime"
import crand "crypto/rand"
import "encoding/base64"
import "sync/atomic"
import "fmt"


func randstring(n int) string {
    b := make([]byte, 2*n)
    crand.Read(b)
    s := base64.URLEncoding.EncodeToString(b)
    return s[0:n]
}

type config struct {
    mu        sync.Mutex
    t         *testing.T
    net       *labrpc.Network
    n         int
    done      int32 // tell internal threads to die
    blockchainPeers     []*Blockchain
    commandChannels     []chan string
    connected []bool        // whether each server is on the net
    endnames  [][]string    // the port file names each sends to
}

var ncpu_once sync.Once

func make_config(t *testing.T, n int) *config {
    ncpu_once.Do(func() {
        if runtime.NumCPU() < 2 {
            fmt.Printf("warning: only one CPU, which may conceal locking bugs\n")
        }
    })
    runtime.GOMAXPROCS(4)
    cfg := &config{}
    cfg.t = t
    cfg.net = labrpc.MakeNetwork()
    cfg.n = n
    cfg.commandChannels = make([]chan string, cfg.n)
    cfg.blockchainPeers = make([]*Blockchain, cfg.n)
    cfg.connected = make([]bool, cfg.n)
    cfg.endnames = make([][]string, cfg.n)

    cfg.setunreliable(false)

    cfg.net.LongDelays(true)

    // create a full set of Blockchain peers.
    for i := 0; i < cfg.n; i++ {
        cfg.start1(i, 2)
    }

    // connect everyone
    for i := 0; i < cfg.n; i++ {
        cfg.connect(i)
    }

    return cfg
}

// shut down a Blockchain server.
func (cfg *config) crash1(i int) {
    cfg.disconnect(i)
    cfg.net.DeleteServer(i) // disable client connections to the server.

    cfg.mu.Lock()
    defer cfg.mu.Unlock()

    bc := cfg.blockchainPeers[i]
    if bc != nil {
        cfg.mu.Unlock()
        bc.Kill()
        cfg.mu.Lock()
        cfg.blockchainPeers[i] = nil
    }
}

func (cfg *config) start1(i int, difficulty int) {
    cfg.crash1(i)

    // a fresh set of outgoing ClientEnd names.
    // so that old crashed instance's ClientEnds can't send.
    cfg.endnames[i] = make([]string, cfg.n)
    for j := 0; j < cfg.n; j++ {
        cfg.endnames[i][j] = randstring(20)
    }

    // a fresh set of ClientEnds.
    ends := make([]*labrpc.ClientEnd, cfg.n)
    for j := 0; j < cfg.n; j++ {
        ends[j] = cfg.net.MakeEnd(cfg.endnames[i][j])
        cfg.net.Connect(cfg.endnames[i][j], j)
    }

    cfg.mu.Lock()

    command := make(chan string)
    cfg.commandChannels[i] = command

    cfg.mu.Unlock()

    bc := Make(ends, i, difficulty, command)

    cfg.mu.Lock()
    cfg.blockchainPeers[i] = bc
    cfg.mu.Unlock()

    svc := labrpc.MakeService(bc)
    srv := labrpc.MakeServer()
    srv.AddService(svc)
    cfg.net.AddServer(i, srv)
}

func (cfg *config) cleanup() {
    for i := 0; i < len(cfg.blockchainPeers); i++ {
        if cfg.blockchainPeers[i] != nil {
            cfg.blockchainPeers[i].Kill()
        }
    }
    atomic.StoreInt32(&cfg.done, 1)
}

// attach server i to the net.
func (cfg *config) connect(i int) {

    cfg.connected[i] = true

    // outgoing ClientEnds
    for j := 0; j < cfg.n; j++ {
        if cfg.connected[j] {
            endname := cfg.endnames[i][j]
            cfg.net.Enable(endname, true)
        }
    }

    // incoming ClientEnds
    for j := 0; j < cfg.n; j++ {
        if cfg.connected[j] {
            endname := cfg.endnames[j][i]
            cfg.net.Enable(endname, true)
        }
    }
}

// detach server i from the net.
func (cfg *config) disconnect(i int) {

    cfg.connected[i] = false

    // outgoing ClientEnds
    for j := 0; j < cfg.n; j++ {
        if cfg.endnames[i] != nil {
            endname := cfg.endnames[i][j]
            cfg.net.Enable(endname, false)
        }
    }

    // incoming ClientEnds
    for j := 0; j < cfg.n; j++ {
        if cfg.endnames[j] != nil {
            endname := cfg.endnames[j][i]
            cfg.net.Enable(endname, false)
        }
    }
}

func (cfg *config) setunreliable(unrel bool) {
    cfg.net.Reliable(!unrel)
}

func (cfg *config) setlongreordering(longrel bool) {
    cfg.net.LongReordering(longrel)
}

func (cfg *config) BroadcastNewBlock(id int) {
    cfg.commandChannels[id] <- "Broadcast"
    status := <- cfg.commandChannels[id]
    if status != "Done" {
        cfg.t.Fatalf("\nError executing BroadcastNewBlock \n")
    }
}

func (cfg *config) DownloadChain(id int) {
    cfg.commandChannels[id] <- "DownloadBlockchain"
    status := <- cfg.commandChannels[id]
    if status != "Done" {
        cfg.t.Fatalf("\nError executing DownloadBlockchain \n")
    }
}
