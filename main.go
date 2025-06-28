package main

import (
	"os"
	"io"
	"fmt"
	"net"
	"log"
	"flag"
	"bytes"
	"strings"

	pb "example.com/meshproxy-go/gomeshproto"
	"google.golang.org/protobuf/proto"

	//mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/acarl005/stripansi"
	bolt "go.etcd.io/bbolt"
)

const start1 = byte(0x94)
const start2 = byte(0xc3)

var portFlag string
var dbDumpFlag bool
var noLogFlag bool
var showPacketsFlag bool
var connections []*net.Conn
var db *bolt.DB

func main() {

	flag.StringVar(&portFlag, "port", "/dev/ttyACM0", "serial port address")
	flag.BoolVar(&dbDumpFlag, "dump", false, "write db contents to stdout")
	flag.BoolVar(&noLogFlag, "nolog", false, "dont disply log output")
	flag.BoolVar(&showPacketsFlag, "showpackets", false, "show packets on output")
  flag.Parse()  // after declaring flags we need to call it

	var err error
	db, err = bolt.Open("database.db", 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()	

	db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("nodes"))
		if err != nil {
			fmt.Printf("create bucket error: %s", err)
			os.Exit(1)
		}
		return nil
	})

	if dbDumpFlag {
		db.View(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte("nodes"))
			if err != nil {
				fmt.Printf("get bucket error: %s", err)
				os.Exit(1)
			}
			c := b.Cursor()
			counter := 0
			for k, v := c.First(); k != nil; k, v = c.Next() {
				counter++
				nodeInfo := pb.NodeInfo{}
				if err := proto.Unmarshal(v, &nodeInfo); err != nil {
					fmt.Printf("Error unmarshalling: %+v\n", err)
				} else {
					fmt.Printf("%s -> %s\n\n", k, nodeInfo.String())
				}
			}
			fmt.Printf("Node count: %d\n", counter)
			return nil
		})
		os.Exit(0)
	}
	
	s := &streamer{}
	s.Init(portFlag)
	defer s.Close()

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

	//opts := mqtt.NewClientOptions()
	//opts.AddBroker("tcp://192.168.1.254:1883")
	//opts.SetClientID("meshproxy-mqtt-go")

	//client := mqtt.NewClient(opts)
	//if token := client.Connect(); token.Wait() && token.Error() != nil {
	//		panic(token.Error())
	//}

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

func sendWantConfigId(s *io.ReadWriteCloser) {

	nodeInfo := pb.ToRadio{PayloadVariant: &pb.ToRadio_WantConfigId{WantConfigId: 42}}

	out, err := proto.Marshal(&nodeInfo)
	if err != nil {
		fmt.Printf("%+v\n", err)
		return
	}

	packageLength := len(string(out))
	header := []byte{start1, start2, byte(packageLength>>8) & 0xff, byte(packageLength) & 0xff}

	radioPacket := append(header, out...)

	//fmt.Printf("%+v\n", radioPacket)
	(*s).Write(radioPacket)
}

//func handleFromRadioStream(s *streamer, mqClient *mqtt.Client) {
func handleFromRadioStream(s *streamer) {
	c, dc := chanFromNode(&s.serialPort)

	for {
		if s.isConnected == false {
			fmt.Printf("Lets reconnect!\n")
			err := s.Reconnect()
			if err != nil {
				continue
			}
			c, dc = chanFromNode(&s.serialPort)
			sendWantConfigId(&s.serialPort)
		}

		select {			
			case logString := <- dc:
				if logString == "SERIALPORTCLOSED" {
					s.isConnected = false
					fmt.Printf("Reconnecting serial port\n")
					err := s.Reconnect()
					if err != nil {
						continue
					}
					c, dc = chanFromNode(&s.serialPort)
					sendWantConfigId(&s.serialPort)
				} else {
					cleanString := strings.TrimSpace(stripansi.Strip(logString))
					if strings.HasSuffix(cleanString, "Lost phone connection") || strings.HasSuffix(cleanString, "Start meshradio init") {
						sendWantConfigId(&s.serialPort)
					}
					if !noLogFlag {
						fmt.Printf("%s", logString)
					}
				}
			case data := <- c:
				//fmt.Printf("%+v\n", data)
				fromRadio := pb.FromRadio{}
				if err := proto.Unmarshal(data[4:], &fromRadio); err != nil {
					fmt.Printf("ERROR decoding packet: %+v\n", err)
				} else if fromRadio.PayloadVariant == nil { 
					fmt.Printf("Could not decode %+v\n", string(data))
				} else {
					if showPacketsFlag {
						fmt.Printf("%+v\n", fromRadio.GetPayloadVariant())
					}
					//(*mqClient).Publish("msh/ANZ/3/e", 0, false, data)

					// send the packet to all TCP connections
					for _, c := range connections {
						(*c).Write(data)
					}
					//fmt.Printf("%T\n", fromRadio.GetPayloadVariant())
					/*if fmt.Sprintf("%T", fromRadio.GetPayloadVariant()) == "*gomeshproto.FromRadio_NodeInfo" {
						nodeInfo := fromRadio.GetNodeInfo()
						if nodeInfo.User == nil {
							continue
						}
						out, err := proto.Marshal(nodeInfo)
						if err != nil {
							fmt.Printf("error %+v\n", err)
							continue
						}						
						//fmt.Printf("%+v\n", out)
						db.Update(func(tx *bolt.Tx) error {
							b := tx.Bucket([]byte("nodes"))
							b.Put([]byte(nodeInfo.User.Id), out)
							return nil
						})
					} else*/ 
					if fmt.Sprintf("%T", fromRadio.GetPayloadVariant()) == "*gomeshproto.FromRadio_Packet" {
						pkt := fromRadio.GetPacket()
						decodedPkt := pkt.GetDecoded()
						if decodedPkt.GetPortnum() == pb.PortNum_NODEINFO_APP {
							userInfo := pb.User{}
							if err := proto.Unmarshal(decodedPkt.GetPayload(), &userInfo); err != nil {
								fmt.Printf("Error unmarshalling: %+v\n", err)
								continue
							}
							nodeInfo := pb.NodeInfo{}
							err = db.View(func(tx *bolt.Tx) error {
								b := tx.Bucket([]byte("nodes"))
								data := b.Get([]byte(userInfo.Id))
								if data != nil {									
									if err := proto.Unmarshal(data, &nodeInfo); err != nil {
										return err
									}
								} else {
									fmt.Printf("New node added to DB %s\n", userInfo.LongName)
								}
								return nil
							})
							if err != nil {
								continue
							}
							
							nodeInfo.Num = pkt.From
							nodeInfo.User = &userInfo
							//fmt.Printf("%s\n", nodeInfo.String())
							// write to db
							out, err := proto.Marshal(&nodeInfo)
							if err != nil {
								fmt.Printf("error %+v\n", err)
								continue
							}						
							//fmt.Printf("%+v\n", out)
							db.Update(func(tx *bolt.Tx) error {
								b := tx.Bucket([]byte("nodes"))
								b.Put([]byte(nodeInfo.User.Id), out)
								return nil
							})
						}
						//fmt.Printf("%+v\n", pkt)
					}
				}
		}
	}
}

func HandleTcpConnection(s *streamer, c *net.Conn) {
	fmt.Printf("Got connection from %s\n", (*c).RemoteAddr())
	connections = append(connections, c)

	b := make([]byte, 1024)
	for {
		n, err := (*c).Read(b)
		if n > 0 {						
			s.serialPort.Write(b)
		}
		if err != nil {
			break
		}
	}

	fmt.Printf("Proxy ended for %s\n", (*c).RemoteAddr())
	for idx, iterConn := range connections {
		if c == iterConn {
			fmt.Printf("Connection removed @ %d\n", idx)
			connections = append(connections[:idx], connections[idx+1:]...)
			break
		}
	}
	
	(*c).Close()
}

func chanFromNode(conn *io.ReadWriteCloser) (chan []byte, chan string) {
	c := make(chan []byte)
	dc := make(chan string)

	const start1 = byte(0x94)
	const start2 = byte(0xc3)
	//emptyByte := make([]byte, 0)

	if conn == nil {
		fmt.Printf("node conn is empty\n")
		os.Exit(0)
	}

	buf := make([]byte, 0)
	logString := make([]byte, 0)

	var pktSize int

	connIface := *conn

	go func() {
		b := make([]byte, 1)
		
		for {
			b = []byte{0x00}
			n, err := connIface.Read(b)
			//fmt.Printf("%+v %+v %+v\n", n, b, err)
			if n > 0 {				

				if len(buf) == 0 && b[0] != start1 {
					logString = append(logString, b[0])
					if len(logString) > 6 && bytes.Equal(logString[len(logString)-6:], []byte{13,10,27,91,48,109}) {
						dc <- string(logString)
						logString = make([]byte, 0)
					}
				}

				if len(buf) == 0 && b[0] == start1 {
					if len(logString) > 0 {
						dc <- string(logString)
						logString = make([]byte, 0)
					}
					buf = append(buf, b...)
					continue
				} else if len(buf) == 1 && b[0] == start2 {
					buf = append(buf, b...)
					continue
				} else if len(buf) == 2 || len(buf) == 3 {
					buf = append(buf, b...)
					continue
				}

				if pktSize == 0 && len(buf) == 4 {
					pktSize = int((buf[2] << 8) + buf[3])
					//fmt.Printf("--> packet header length %d\n", pktSize)
				}

				if pktSize > 0 {
					buf = append(buf, b...)

					if len(buf) >= pktSize + 4 {
						//fmt.Printf("full packet found %+v\n", buf)
						res := make([]byte, len(buf))
						copy(res, buf)
						c <- res
						buf = make([]byte, 0)
						pktSize = 0
					}
				}
			}
			if err != nil {
				dc <- "SERIALPORTCLOSED"
				break
			}
		}

		fmt.Printf("Exited serial read loop\n")
	}()

	return c, dc
}