package main

import (
  "os"
  "fmt"
  "net"
  "log"
  "flag"
  "syscall"
  "os/signal"	
)

var portFlag string
var dbDumpFlag string
var logFlag bool
var showPacketsFlag bool
var indexFlag bool
var removeNodeFlag string
var deviceNodesFlag bool

var shuttingDown = false

func main() {

  flag.StringVar(&portFlag, "port", "", "serial port address")
  flag.StringVar(&dbDumpFlag, "dump", "", "write db contents to stdout (nodes,nodeindex,messages)")
  flag.StringVar(&removeNodeFlag, "removenode", "", "remove node from db given 8 character hex string")
  flag.BoolVar(&logFlag, "log", false, "show device log output")
  flag.BoolVar(&showPacketsFlag, "showpackets", false, "show packets on output")
  flag.BoolVar(&deviceNodesFlag, "devicenodes", false, "use device nodes instead of local db")
  //flag.BoolVar(&indexFlag, "index", false, "index nodes")
  flag.Parse()  // after declaring flags we need to call it

  s := &streamer{}
  l, err := net.Listen("tcp", ":4403")
  if err != nil {
    log.Fatal(err)
  }	
  defer l.Close()

  shutdownDone := ListenForShutdown(s, l)
  InitDb()

  if len(dbDumpFlag) > 0 {
    DumpDb(dbDumpFlag)
    os.Exit(0)
  }

  if indexFlag {
    IndexDb()
    os.Exit(0)
  }

  if len(removeNodeFlag) > 0 {
    RemoveNodeFromDb(removeNodeFlag)
    os.Exit(0)
  }
  
  s.Init(portFlag)

  if s.serialPort == nil {
    fmt.Printf("Serial port failure\n")
    os.Exit(1)
  }
  fmt.Printf("Serial port connected\n")

  //go handleFromRadioStream(s, &client)
  go handleFromRadioStream(s)
  sendWantConfigId(&s.serialPort)

  fmt.Printf("Listening on 4403\n")
  for {
    var err error
    activeConn, err := l.Accept()
    if shuttingDown {
      break
    }
    if err != nil {
      log.Fatal(err)
    }		
    go HandleTcpConnection(s, activeConn)
  }	

  <- shutdownDone
}

func ListenForShutdown(s *streamer, l net.Listener) chan bool {
  d := make(chan bool)
  c := make(chan os.Signal)
  signal.Notify(c, os.Interrupt, syscall.SIGTERM)
  go func() {
    <-c
    fmt.Printf("\nExiting...\n")
    shuttingDown = true
    l.Close()
    fmt.Printf("Closing TCP Connections\n")
    for _, c := range connections {
      c.Close()
    }

    //time.Sleep(10 * time.Second)
    fmt.Printf("Closing Serial connection\n")
    s.Close()

    fmt.Printf("Closing Db\n")
    CloseDb()
    d <- true
  }()
  return d
}
