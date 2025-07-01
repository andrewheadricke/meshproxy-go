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
var noLogFlag bool
var showPacketsFlag bool
var indexFlag bool

func main() {

	flag.StringVar(&portFlag, "port", "/dev/ttyACM0", "serial port address")
	flag.StringVar(&dbDumpFlag, "dump", "", "write db contents to stdout (nodes,nodeindex)")
	flag.BoolVar(&noLogFlag, "nolog", false, "dont disply log output")
	flag.BoolVar(&showPacketsFlag, "showpackets", false, "show packets on output")
	//flag.BoolVar(&indexFlag, "index", false, "index nodes")
  flag.Parse()  // after declaring flags we need to call it

	s := &streamer{}
	ListenForShutdown(s)
	InitDb()

	if len(dbDumpFlag) > 0 {
		DumpDb(dbDumpFlag)
		os.Exit(0)
	}

	if indexFlag {
		IndexDb()
		os.Exit(0)
	}
	
	s.Init(portFlag)

	l, err := net.Listen("tcp", ":4403")
	if err != nil {
		log.Fatal(err)
	}
	defer l.Close()
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
		if err != nil {
			log.Fatal(err)
		}
		go HandleTcpConnection(s, &activeConn)
	}	
}

func ListenForShutdown(s *streamer) {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Printf("\nExiting...\n")
		fmt.Printf("Closing TCP Connections\n")
		for _, c := range connections {
			(*c).Close()
		}

		fmt.Printf("Closing Serial connection\n")
		s.Close()

		fmt.Printf("Closing Db\n")
		CloseDb()
		os.Exit(0)
	}()
}