package blockchain

// Blockchain tests.
//
// we will use the original test_test.go to test your code for grading.
// so, while you can modify this code to help you debug, please
// test with the original before submitting.
//

import "testing"
import "fmt"
import "time"
import "sync"

const BroadcastTime = 1000 * time.Millisecond

func TestInitialState4(t *testing.T) {
    servers := 3
    cfg := make_config(t, servers)
    defer cfg.cleanup()

    fmt.Printf("\nTest : initial state ...\n")
    fmt.Printf("\nInitiating nodes...\n")

    max := 0

    for i := 0; i < servers; i++ {
        repLength, _ := cfg.blockchainPeers[i].GetState()

        fmt.Printf("\nLength of Node : %v is %v\n", i, repLength)

        l := repLength

        if max < l {
            max = l
        }
    }

    if max != 1 {
        t.Fatalf("\nWarning : Initial state error, length should be 1.\n")
    }

    fmt.Printf("\n  ... Passed\n")
}


func TestBasicMine4(t *testing.T) {
    servers := 3
    cfg := make_config(t, servers)
    defer cfg.cleanup()

    fmt.Printf("\nTest: one mining request ...\n")

    var args CreateNewBlockArgs
    var reply CreateNewBlockReply

    args.BlockData = "Node0 mine"
    //fmt.Println("line 58")
    cfg.blockchainPeers[0].CreateNewBlock(&args, &reply)
    //fmt.Println("line 60")
    len, _ := cfg.blockchainPeers[0].GetState()
    //fmt.Println("line 62")

    if len >= 2 {
        t.Fatalf("\nWarning: should not add block to chain before broadcast")
    }

    cfg.BroadcastNewBlock(0)

    time.Sleep(BroadcastTime)

    for i := 0; i < servers; i++ {
        repLength, _ := cfg.blockchainPeers[i].GetState()

        fmt.Printf("\nLength of Node : %v is %v\n", i, repLength)

        l := repLength

        if l != 2 {
            t.Fatalf("\nWarning : Length should be 2.\n")
        }
    }

    fmt.Printf("\n  ... Passed\n")
}

func TestBasicMultiMining4(t *testing.T) {

    servers := 3
    cfg := make_config(t, servers)
    defer cfg.cleanup()

    fmt.Printf("\nTest: Multi mining requests ...\n")

    var cnt int = 6
    for node := 0; node < cnt; node++ {
        id := node % servers

        var args CreateNewBlockArgs
        var reply CreateNewBlockReply

        args.BlockData = fmt.Sprintf("%s %d %s", "Node", id, "Mine")

        cfg.blockchainPeers[id].CreateNewBlock(&args, &reply)

        cfg.BroadcastNewBlock(id)

        time.Sleep(BroadcastTime)
    }

    for i := 0; i < servers; i++ {
        repLength, _ := cfg.blockchainPeers[i].GetState()

        fmt.Printf("\nLength of Node : %v is %v\n", i, repLength)

        l := repLength

        if l != cnt + 1 {
            t.Fatalf("\nWarning : Warning : Broadcast error. Wrong Length !\n")
        }
    }

    fmt.Printf("\n  ... Passed\n")
}

func TestConsensus4(t *testing.T) {
    servers := 4
    cfg := make_config(t, servers)
    defer cfg.cleanup()

    fmt.Printf("\nTest: consensus within multiple nodes ...\n")

    cnt := 4
    var wg sync.WaitGroup

    for i := 0; i < cnt; i++ {
        wg.Add(1)

        go func(i int) {
            defer wg.Done()

            var args CreateNewBlockArgs
            var reply CreateNewBlockReply

            args.BlockData = fmt.Sprintf("%s %d %s", "Thread Node", i, "Mine")
            fmt.Printf("%s %d %s\n", "Thread Node", i, "Mine")
            cfg.blockchainPeers[i].CreateNewBlock(&args, &reply)

            cfg.BroadcastNewBlock(i)

            time.Sleep(BroadcastTime)
        }(i)
    }

    wg.Wait()

    var lastHash string

    for i := 0; i < servers; i++ {
        repLength, repHash := cfg.blockchainPeers[i].GetState()

        fmt.Printf("Length of Node : %v is %v\n", i, repLength)

        l := repLength

        if l != 2 {
            t.Fatalf("Warning : Length should be 2.\n")
        }

        if len(lastHash) == 0 {
            lastHash = repHash
        } else {
            if lastHash != repHash {
                t.Fatalf("No consensus found for %v and %v", lastHash, repHash)
            }
        }
    }

    fmt.Printf("  ... Passed\n")
}


func TestReconnectConsensus4(t *testing.T) {

    servers := 4
    cfg := make_config(t, servers)
    defer cfg.cleanup()

    fmt.Printf("\nTesting consensus after disconnection. ...\n")
    fmt.Printf("Initiating nodes ...")

    cfg.disconnect(3);

    time.Sleep(BroadcastTime)
    cnt := 2
    id := 0

    for node := 0; node < cnt; node++ {

        var args CreateNewBlockArgs
        var reply CreateNewBlockReply

        args.BlockData = fmt.Sprintf("%s %d %s", "Node", id, "Mine")

        cfg.blockchainPeers[id].CreateNewBlock(&args, &reply)

        cfg.BroadcastNewBlock(id)

        time.Sleep(BroadcastTime)
    }

    time.Sleep(BroadcastTime)

    fmt.Printf(" \n Check length and hash... \n")
    var lastHash string

    for i := 0; i < servers - 1; i++ {

        repLength, repHash := cfg.blockchainPeers[i].GetState()

        fmt.Printf("\nLength of Node : %v is %v\n", i, repLength)

        l := repLength

        if l != cnt+1 {
            t.Fatalf("\nWarning : wrong Length !\n")
        }

        if len(lastHash) == 0 {
            lastHash = repHash
        } else {
            if lastHash != repHash {
                t.Fatalf("\nNo consensus found for %v and %v", lastHash, repHash)
            }
        }
    }

    fmt.Printf(" \n Reconnect server 3... \n")

    cfg.connect(3);

    cfg.DownloadChain(3);

    time.Sleep(BroadcastTime)

    fmt.Printf(" \n Check final status... \n")

    newLength, newLastHash := cfg.blockchainPeers[3].GetState()

    if newLength != cnt+1 {
        t.Fatalf("\nError: Final length wrong ! ")
    }

    if lastHash != newLastHash {
        t.Fatalf(" \nError Final hash wrong \n" )
    }

    fmt.Printf("  ... Passed\n")
}
