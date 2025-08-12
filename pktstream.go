package main

import (
  "io"
  "fmt"
  "log"
  "strings"
  "strconv"
  "encoding/binary"

  bolt "go.etcd.io/bbolt"
  "github.com/acarl005/stripansi"
  "google.golang.org/protobuf/proto"
  pb "example.com/meshproxy-go/gomeshproto"
)

type MsgBytes []byte

var handshakeMessages = []MsgBytes{}
var capturingHandshake = false
var firstNodeInfoReceived = false
var connectedNodeId = ""

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

  fmt.Printf("Capturing handshake packets...\n")
  capturingHandshake = true
  firstNodeInfoReceived = false
  handshakeMessages = []MsgBytes{}
  (*s).Write(radioPacket)
}

//func handleFromRadioStream(s *streamer, mqClient *mqtt.Client) {
func handleFromRadioStream(s *streamer) {
  c, dc := chanFromNode(&s.serialPort)

  for {
    if s.isConnected == false && s.dontReconnect == false {
      fmt.Printf("Lets reconnect!\n")
      err := s.Reconnect()
      if err != nil {
        continue
      }
      c, dc = chanFromNode(&s.serialPort)
      sendWantConfigId(&s.serialPort)
    }

    select {			
      // dc = debug logging channel
      case logString := <- dc:
        if logString == "SERIALPORTCLOSED" {
          s.isConnected = false
          if s.dontReconnect == false {
            fmt.Printf("Reconnecting serial port\n")
            err := s.Reconnect()
            if err != nil {
              continue
            }
            c, dc = chanFromNode(&s.serialPort)
            sendWantConfigId(&s.serialPort)
          }
        } else {
          cleanString := strings.TrimSpace(stripansi.Strip(logString))
          if strings.HasSuffix(cleanString, "Lost phone connection") || strings.HasSuffix(cleanString, "Start meshradio init") {
            sendWantConfigId(&s.serialPort)
          }
          if logFlag {
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
          if !capturingHandshake {
            BroadcastMessageToConnections(data)
          }
          HandlePackets(&fromRadio, data)
        }
    }
  }
}

func HandlePackets(fromRadio *pb.FromRadio, rawBytes []byte) {
  //fmt.Printf("fromRadio type: %T\n", fromRadio.GetPayloadVariant())
  if fmt.Sprintf("%T", fromRadio.GetPayloadVariant()) == "*gomeshproto.FromRadio_ConfigCompleteId" {
    fmt.Printf("Handshake capture complete, %d messages\n", len(handshakeMessages))
    capturingHandshake = false
  } else if fmt.Sprintf("%T", fromRadio.GetPayloadVariant()) == "*gomeshproto.FromRadio_Packet" {

    pkt := fromRadio.GetPacket()
    decodedPkt := pkt.GetDecoded()

    isNewNode := false
    shouldUpdateIndex := false
    nodeInfo, err := FetchNodeInfoByNumber(pkt.From)
    nodeId := fmt.Sprintf("!%08x", pkt.From)

    if err != nil {
      if err.Error() != "node not found" {
        log.Fatal(err)
      }

      isNewNode = true			
      nodeInfo = pb.NodeInfo{}

      if decodedPkt.GetPortnum() != pb.PortNum_NODEINFO_APP {
        nodeInfo.User = &pb.User{
          Id: nodeId, 
          ShortName: nodeId[len(nodeId)-4:], 
          LongName: "Meshtastic " + nodeId[len(nodeId)-4:],
        }
        fmt.Printf("Added new unknown node %s\n", nodeInfo.User.LongName)
      }
        
    }

    nodeInfo.Num = pkt.From
    nodeInfo.LastHeard = pkt.RxTime
    hops := pkt.HopStart - pkt.HopLimit
    //hops := uint32(0)
    nodeInfo.HopsAway = &hops

    if decodedPkt.GetPortnum() == pb.PortNum_TEXT_MESSAGE_APP {

      if len(connections) == 0 {
        fmt.Printf("Saving TEXT_MESSAGE: %s\n", string(decodedPkt.GetPayload()))
        AppendMessagePktToDb(pkt.RxTime, rawBytes)
      }

    } else if decodedPkt.GetPortnum() == pb.PortNum_NODEINFO_APP {
      userInfo := pb.User{}
      if err := proto.Unmarshal(decodedPkt.GetPayload(), &userInfo); err != nil {
        fmt.Printf("Error unmarshalling: %+v\n", err)
        return
      }
      
      if !isNewNode && userInfo.LongName != nodeInfo.User.LongName {
        fmt.Printf("Node %s renamed to %s\n", nodeInfo.User.LongName, userInfo.LongName)
      }

      //fmt.Printf("%+v %+v - %+v %+v\n", pkt.From, nodeId, userInfo, nodeInfo)

      if userInfo.Id != nodeId {
        defaultNodeNumber, _ := strconv.ParseUint(userInfo.Id[1:], 16, 64)
        fmt.Printf("Non default node number detected, using %d instead of %d for %s - %s\n", pkt.From, defaultNodeNumber, userInfo.Id, userInfo.LongName)
        shouldUpdateIndex = true
      }

      nodeInfo.User = &userInfo
    } else if decodedPkt.GetPortnum() == pb.PortNum_POSITION_APP {
      posInfo := pb.Position{}
      if err := proto.Unmarshal(decodedPkt.GetPayload(), &posInfo); err != nil {
        fmt.Printf("Error unmarshalling: %+v\n", err)
        return
      }

      nodeInfo.Position = &posInfo

      fmt.Printf("Position updated for %s\n", nodeInfo.User.LongName)
    } else if decodedPkt.GetPortnum() == pb.PortNum_UNKNOWN_APP {
      
    } else if decodedPkt.GetPortnum() == pb.PortNum_ROUTING_APP || decodedPkt.GetPortnum() == pb.PortNum_TELEMETRY_APP {
      
    } else {
      fmt.Printf("Got portnum %d\n", decodedPkt.GetPortnum())
    }

    // write to db
    out, err := proto.Marshal(&nodeInfo)
    if err != nil {
      fmt.Printf("error %+v\n", err)
      return
    }
    //fmt.Printf("%+v\n", out)
    fmt.Printf("Updating node %s\n", nodeInfo.User.LongName)
    db.Update(func(tx *bolt.Tx) error {
      b := tx.Bucket([]byte("nodes"))
      b.Put([]byte(nodeInfo.User.Id), out)

      if isNewNode || shouldUpdateIndex {
        b := tx.Bucket([]byte("nodeindex"))
        nodeNumBytes := make([]byte, 4)
        binary.LittleEndian.PutUint32(nodeNumBytes, nodeInfo.Num)
        b.Put(nodeNumBytes, []byte(nodeInfo.User.Id))
      }

      return nil
    })

  } else {
    if capturingHandshake {
      if fmt.Sprintf("%T", fromRadio.GetPayloadVariant()) == "*gomeshproto.FromRadio_NodeInfo" {
        if !firstNodeInfoReceived {
          nodeInfo := fromRadio.GetNodeInfo()
          connectedNodeId = nodeInfo.User.Id
          SaveNodeToDb(nodeInfo)
          // dont return, so we append the firstNode to handshake list
          firstNodeInfoReceived = true
        } else {
          if !deviceNodesFlag {
            return
          }
        }
      }
      handshakeMessages = append(handshakeMessages, rawBytes)
    }
  }
}

// Code to handle NodeInfo during handshake
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
}*/