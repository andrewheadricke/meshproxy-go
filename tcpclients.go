package main

import (
  "fmt"
  "net"
  "time"
  "bytes"
  "errors"
  "strings"
  //"crypto/rand"

  "github.com/brucespang/go-tcpinfo"
  "google.golang.org/protobuf/proto"
  pb "example.com/meshproxy-go/gomeshproto"

)

var connections []net.Conn

func HandleTcpConnection(s *streamer, c net.Conn) {
  fmt.Printf("Got connection from %s\n", c.RemoteAddr())
  connections = append(connections, c)
    
  handshakeComplete := false
  buf := make([]byte, 0)

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

    n, err := c.Read(b)
    if n > 0 {      
      if handshakeComplete {
        s.serialPort.Write(b)
      } else {
        buf = append(buf, b[:n]...)
        if err := CheckAndSendClientHandshake(buf, c); err == nil {
          //fmt.Printf("client handshake complete\n")
          handshakeComplete = true
        }	
      }
    }
    if err != nil {
      if strings.HasSuffix(err.Error(), "use of closed network connection") || err.Error() == "EOF" {
      } else {
        fmt.Printf("unexpected read error: %+v\n", err)
      }
      break
    }
  }

  fmt.Printf("Proxy ended for %s\n", c.RemoteAddr())
  for idx, iterConn := range connections {
    if c == iterConn {
      fmt.Printf("Connection removed @ %d\n", idx)
      connections = append(connections[:idx], connections[idx+1:]...)
      break
    }
  }
  
  c.Close()
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
  
  //s.serialPort.Write(buf[pos:pos+6])
  for _, msgBytes := range handshakeMessages {
    //fmt.Printf("sending %+v\n", msgBytes)
    c.Write(msgBytes)
  }

  // SEND THE NODES
  if !deviceNodesFlag {
    IterateNodesFromDb(func(n []byte) {
      packageLength := len(string(n))
      header := []byte{start1, start2, byte(packageLength>>8) & 0xff, byte(packageLength) & 0xff}
      radioPacket := append(header, n...)
      c.Write(radioPacket)		
    })
  }

  ccid := pb.FromRadio{PayloadVariant: &pb.FromRadio_ConfigCompleteId{ConfigCompleteId: wantConfigId}}
  out, err := proto.Marshal(&ccid)
  if err != nil {
    return err
  }

  packageLength := len(string(out))
  header := []byte{start1, start2, byte(packageLength>>8) & 0xff, byte(packageLength) & 0xff}
  radioPacket := append(header, out...)

  //fmt.Printf("sending final %+v\n", radioPacket)
  c.Write(radioPacket)

  // now send some historical messages
  IterateMessagesFromDb(func(pkt []byte){
    //fmt.Printf("sending old pkt %+v\n", pkt)
    c.Write(pkt)
  })

  return nil
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
      fmt.Printf("Closed connection %s\n", c.RemoteAddr())
      c.Close()
      continue
    }

    tcpConn := c.(*net.TCPConn)
    tcpInfo, err := tcpinfo.GetsockoptTCPInfo(tcpConn)
    if err != nil {
      fmt.Printf("looks like connection is closed\n")
      continue
    }
    if tcpInfo.Retransmits > 5 {
      fmt.Printf("Closing broken connection @ %s\n", c.RemoteAddr())
      c.Close()
    }
  }
}