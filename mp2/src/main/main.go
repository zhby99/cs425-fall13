package main

import (
    "flag"
    "io"
    "leader"
    "log"
    "membertable"
    "net"
    "os"
    "net/rpc"
    "net/http"
    "strconv"
)

var listenAddress = flag.String("bind", ":7777", "the address for listening")
var leaderAddress = flag.String("leader", "", "the address of the leader machine; leave unset to make this process leader")
var seedAddress = flag.String("seed", "", "the address of some machine to grab the inital membertable from")
var machineName = flag.String("name", "", "the name of this machine")
var logFile = flag.String("logs", "machine.log", "the file name to store the log in")


func leaderProcess(fatalChan chan bool) {
    if err := leader.Run(); err != nil {
        log.Fatal(err)
    }
    fatalChan <- true
}

func getIP(hostname string) string {
    machineIP, err := net.InterfaceAddrs()
    if err != nil || len(machineIP) == 0 {
        return ""
    }

    var preferredIP net.IP
    for _, ipStr := range machineIP {
        ipAddr, ok := ipStr.(*net.IPNet)
        if !ok {
            continue
        }

        ip := ipAddr.IP

        // Prefer IPv4 addresses that come sooner in the list and are not local of LookupHost
        if preferredIP == nil || (preferredIP.To4() == nil && ip.To4() != nil) || (preferredIP[12] == 127) {
            preferredIP = ip
        }
    }

    return preferredIP.String()
}

// Choose a color for a given ID
// TODO maybe move this to membertable
func getColor(id membertable.ID) string {
    switch id.Num % 6 {
        case 0: return "1;31";
        case 1: return "1;32";
        case 2: return "1;34";
        case 3: return "1;33";
        case 4: return "1;35";
        case 5: return "1;36";
    }
    return "0";
}

func main() {
    flag.Parse()

    fatalChan := make(chan bool)

    // Get the machines name
    hostname, _ := os.Hostname()

    // Get the address that this machine can be contacted from if none was given
    bindAddress, bindPort, err := net.SplitHostPort(*listenAddress)
    if bindAddress == "" {
        bindAddress = getIP(hostname)
    }


    var id membertable.IDNum

    if *leaderAddress == "" {
        // We are the LEADER! Take an ID and take our role as Master of IDs.
        if id, err = leader.IncrementIDFile(); err != nil {
            log.Fatal(err)
            return
        }
        go leaderProcess(fatalChan)
    } else {
        // Get an ID from the leader
        if id, err = leader.RequestID(*leaderAddress); err != nil {
            log.Fatal(err)
            return
        }
    }



    // Add ourselves to the table
    myID := membertable.ID{
        Num: id,
        Name: *machineName,
        Address: bindAddress + ":" + bindPort,
    }

    me := membertable.Member{
        ID: myID,
        HeartbeatID: 0,
    }

    // If no name was given, default to the host name
    if me.ID.Name == "" {
        me.ID.Name = hostname
    }

    var t membertable.Table
    t.Init(me.ID)

    // Configure the log file to be something nice
    log.SetPrefix("[\x1B[" + getColor(me.ID) + "m" + me.ID.Name + " " + strconv.Itoa(int(me.ID.Num)) + " " + bindAddress + "\x1B[0m]:")
    //log.SetPrefix(strconv.Itoa(int(me.ID)) + " " + bindAddress + ":")
    log.SetFlags(0)

    logfd, err := os.Create(*logFile + me.ID.Name)

    if err != nil {
        log.Println(err)
    }

    log.SetOutput(io.MultiWriter(logfd, os.Stdout))
    log.Println("Hostname :", hostname)
    log.Println("Name     :", me.ID.Name)
    log.Println("IP       :", bindAddress)
    log.Println("Address  :", me.ID.Address)
    log.Println("ID       :", me.ID.Num)

    if *seedAddress != "" {
        log.Printf("sending heartbeat to seed member")
        if err = t.SendHeartbeatToAddress(*seedAddress); err != nil {
            log.Fatal(err)
        }
    }

    rpc.Register(&t)
    rpc.HandleHTTP()
    l, e := net.Listen("tcp", ":" + bindPort)
    log.Print("Bindport: " + bindPort)
    if e != nil {
        log.Print("RPC bind failure")
    }
    go http.Serve(l, nil)
    go t.SendHeartbeatProcess(fatalChan)

    <-fatalChan
}
