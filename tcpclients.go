package main

import (
  "io"
  "fmt"
  "net"
  "time"
  "sync"
  "bytes"
  //"bufio"
  "errors"	
  "strings"
  "encoding/json"
  "encoding/base64"

  "github.com/gorilla/websocket"
  "github.com/jursonmo/go-tcpinfo"
  "google.golang.org/protobuf/proto"
  pb "example.com/meshproxy-go/gomeshproto"
)

// we mainly need this for broadcasting / sending out data from the radio to clients
type ClientConnection struct {
  connType int // 1 = tcp, 2 = websocket, 3 = http request
  rawConn net.Conn
  webSocket *websocket.Conn
  mu sync.Mutex
}

func (c *ClientConnection) Close() {
  if c.connType == 1 {
    c.rawConn.Close()
  } else {
    c.webSocket.Close()
  }
}

func (c *ClientConnection) Write(data []byte) (size int, err error) {
  if c.connType == 1 {
    return c.rawConn.Write(data)
  } else {
    response := map[string]interface{}{}
    response["protobuf"] = base64.StdEncoding.EncodeToString(data)
    response["type"] = "from_radio"

    fromRadio := pb.FromRadio{}
    if err := proto.Unmarshal(data[4:], &fromRadio); err != nil {
      fmt.Printf("%+v\n", err)			
    } else {
      response["json"] = fromRadio.PayloadVariant
    }
    
    responseBytes, _ := json.Marshal(response)

    c.mu.Lock()
    err := c.webSocket.WriteMessage(1, responseBytes)
    c.mu.Unlock()
    if err != nil {
      return len(data), nil
    } else {
      return 0, err
    }		
  }
}

func (c *ClientConnection) WriteWsResponse(message string) {
  if c.connType == 1 {
    return
  }

  data := []byte(message)

  c.webSocket.WriteMessage(1, data)
}


var connections []*ClientConnection

func HandleTcpConnection(s *streamer, conn net.Conn) {
  fmt.Printf("Got connection from %s\n", conn.RemoteAddr())

  newConn := ClientConnection{connType: 1, rawConn: conn}
  connections = append(connections, &newConn)
    
  handshakeComplete := false
  buf := make([]byte, 0)
  chanWsComplete := make(chan int)

  // wait for WantConfig, keep request Id
  // send cached handshakeMessages (stop after PAX)
  // iterate nodes in local db, send them
  // send remaining cached handshakeMessages
  // send ConfigCompleteId: [requestId]
  // switch to full proxy mode

  b := make([]byte, 1024)
  for {

    for {
      // dodgy hack to wait for serial port handshake to complete
      if !capturingHandshake {
        break
      }
      if shuttingDown {
        break
      }
      time.Sleep(50 * time.Millisecond)
    }

    n, err := newConn.rawConn.Read(b)
    if err != nil {
      if strings.HasSuffix(err.Error(), "use of closed network connection") || err.Error() == "EOF" {
      } else {
        fmt.Printf("unexpected read error: %+v\n", err)
      }
      break
    }
    if n > 0 {
      tmpBuf := make([]byte, n)
      copy(tmpBuf, b[:n])

      if handshakeComplete {
        s.Write(tmpBuf)
      } else {
        buf = append(buf, tmpBuf...)

				var connType int
        connType, chanWsComplete = CheckForHttpRequest(buf, &newConn, s)
        if connType == 2 {
          fmt.Printf("Switching to Websocket\n")
          newConn.connType = 2
          //fmt.Printf("%+v\n", *connections[0])
          break
        } else if connType == 3 {
					//fmt.Printf("Detected HTTP request\n")
					break
				}

        if err := CheckAndSendClientHandshake(buf, newConn.rawConn); err == nil {
          //fmt.Printf("client handshake complete\n")
          handshakeComplete = true
        }	
      }
    }
  }

  if chanWsComplete != nil {
    //fmt.Printf("waiting for websocket completion\n")
    <- chanWsComplete
  }

  fmt.Printf("Proxy ended for %s\n", newConn.rawConn.RemoteAddr())
  for idx, iterConn := range connections {
    if newConn.rawConn == iterConn.rawConn {
      fmt.Printf("Connection removed @ %d\n", idx)
      connections = append(connections[:idx], connections[idx+1:]...)
      break
    }
  }
  
  newConn.Close()
}

func CheckAndSendClientHandshake(buf []byte, c net.Conn) error {
  pos := bytes.Index(buf, []byte{start1, start2})
  if pos == -1 || len(buf) < pos + 2 {
    return errors.New("not found")
  }

  pktSize := int((buf[pos+2] << 8) + buf[pos+3])
  if len(buf) < pos + 4 + pktSize {
    return errors.New("not found")
  }

  toRadio := pb.ToRadio{}
  if err := proto.Unmarshal(buf[pos+4:pos+4+pktSize], &toRadio); err != nil {
    return errors.New("decode handshake failed")
  }
  if fmt.Sprintf("%T", toRadio.GetPayloadVariant()) != "*gomeshproto.ToRadio_WantConfigId" {
    return errors.New("not WantConfigId")
  }
  wantConfigId := toRadio.GetWantConfigId()
  
  SendHandshakeMessages(c, wantConfigId)
  
  return nil
}

func SendHandshakeMessages(w io.Writer, wantConfigId uint32) {
  //s.serialPort.Write(buf[pos:pos+6])
  for _, msgBytes := range handshakeMessages {
    //fmt.Printf("sending %+v\n", msgBytes)
    w.Write(msgBytes)
  }

  // SEND THE NODES
  if !deviceNodesFlag {
    IterateNodesFromDb(func(n []byte) {
      packageLength := len(string(n))
      header := []byte{start1, start2, byte(packageLength>>8) & 0xff, byte(packageLength) & 0xff}
      radioPacket := append(header, n...)
      w.Write(radioPacket)		
    })
  }

  ccid := pb.FromRadio{PayloadVariant: &pb.FromRadio_ConfigCompleteId{ConfigCompleteId: wantConfigId}}
  out, err := proto.Marshal(&ccid)
  if err != nil {
    fmt.Printf("Error marshalling ConfigCompleteId\n")
    return
  }

  packageLength := len(string(out))
  header := []byte{start1, start2, byte(packageLength>>8) & 0xff, byte(packageLength) & 0xff}
  radioPacket := append(header, out...)

  //fmt.Printf("sending final %+v\n", radioPacket)
  w.Write(radioPacket)

  // now send some historical messages
  IterateMessagesFromDb(func(pkt []byte){
    //fmt.Printf("sending old pkt %+v\n", pkt)
    w.Write(pkt)
  })
}

func BroadcastMessageToConnections(messageData []byte) {

  //fmt.Printf("broadcasting message\n")
  for _, c := range connections {

    if shuttingDown {
      return
    }

    //(*c).SetDeadline(time.Now().Add(1 * time.Second))
    _, err := c.Write(messageData)
    if err != nil {
      fmt.Printf("Closed connection %s\n", c.rawConn.RemoteAddr())
      c.Close()
      continue
    }

    tcpConn := c.rawConn.(*net.TCPConn)
    tcpInfo, err := tcpinfo.GetTCPInfo(tcpConn)
    if err != nil {
      fmt.Printf("GetTCPInfo error: %+v\n", err)
      c.Close()
      continue
    }
    if tcpInfo.Retransmits > 5 {
      fmt.Printf("Closing broken connection @ %s\n", c.rawConn.RemoteAddr())
      c.Close()
    }
  }
}