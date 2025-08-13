package main

import (
	"fmt"
	"log"	
	"time"
	"bytes"
	"bufio"
	"errors"
	"regexp"
	"net/http"
	"math/rand"
	"encoding/json"
	"encoding/base64"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
  pb "example.com/meshproxy-go/gomeshproto"
)

var chanWebsocketComplete chan bool

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
 },
}

const broadcastNum = 0xffffffff

func CheckForHttpRequest(buf []byte, clientConn *ClientConnection, s *streamer) (connType int, wsComplete chan int) {
	//fmt.Printf("%+v\n%+v\n", buf, string(buf))

	re := regexp.MustCompile(`GET (/[a-zA-Z0-9_\-\/\.]*) HTTP/1.1`)
	match := re.FindStringSubmatch(string(buf))

	if len(match) == 2 {
		//fmt.Printf("http request detected\n")

		req := getHttpRequest(buf)
		rw := NewCustomResponseWriter(clientConn.rawConn)

		if req.Header.Get("Upgrade") != "" {
			return 2, HandleWebsocketUpgrade(req, rw, clientConn, s)
		} else {
			if subFS == nil {
				http.NotFound(rw, req)
			} else {
				http.ServeFileFS(rw, req, subFS, match[1])
			}
			return 3, nil
		}		
	}

	return 1, nil
}

func HandleWebsocketUpgrade(req *http.Request, rw *CustomResponseWriter, clientConn *ClientConnection, s *streamer) chan int {
	wsConn, err := upgrader.Upgrade(rw, req, nil)	
	if err != nil {
			log.Println(err)
			return nil
	}

	//fmt.Printf("%+v\n", wsConn)
	clientConn.webSocket = wsConn
	chanWsComplete := make(chan int)

	go func() {
		for {
			_, data, err := wsConn.ReadMessage()
			if err != nil {
				wsConn.Close()
				chanWsComplete <- 1
				return
			}
			EncodeProtobufAndSend(data, s, clientConn)
		}
	}()

	return chanWsComplete
}

func getHttpRequest(readBuf []byte) *http.Request {
	//defer conn.Close()

	reader := bufio.NewReader(bytes.NewReader(readBuf))

	// Read the HTTP request from the reader
	req, err := http.ReadRequest(reader)
	if err != nil {
		fmt.Printf("Error reading HTTP request: %v\n", err)
		return nil
	}
	return req
}

type toRadio struct {
	Type string
	Json map[string]interface{}
}

func EncodeProtobufAndSend(msg []byte, s *streamer, clientConn *ClientConnection) {	
	msgJson := toRadio{}
	err := json.Unmarshal(msg, &msgJson)
	if err != nil {
		fmt.Printf("could not unmarshal json\n")
		return
	}

	if wantConfigIface, bFound := msgJson.Json["WantConfigId"]; bFound {
		wantConfigMap := wantConfigIface.(map[string]interface{})
		wantConfigId := uint32(wantConfigMap["ConfigId"].(float64))
		
		SendHandshakeMessages(clientConn, wantConfigId)
	} else if sendPacketIface, bFound := msgJson.Json["SendPacket"]; bFound {		
		sendPacketMap, ok := sendPacketIface.(map[string]interface{})
		if !ok {
			clientConn.WriteWsResponse("invalid packet type")
			return
		}
		var payload []byte
		var portnum float64
		var to float64
		var channel float64

		payloadB64, ok := sendPacketMap["payload_b64"].(string)
		if !ok {
			clientConn.WriteWsResponse("invalid payload type")
			return
		}
		payload, err = base64.StdEncoding.DecodeString(payloadB64)
		if err != nil {
			clientConn.WriteWsResponse("invalid payload base64")
			return
		}

		portnum, ok = sendPacketMap["portnum"].(float64)
		if !ok {
			clientConn.WriteWsResponse("invalid portnum type")
			return
		}

		to, ok = sendPacketMap["to"].(float64)
		if !ok {
			clientConn.WriteWsResponse("invalid to type")
			return
		}

		channel, ok = sendPacketMap["channel"].(float64)
		if !ok {
			clientConn.WriteWsResponse("invalid channel type")
			return
		}

		err := SendPacket(payload, pb.PortNum(portnum), int64(to), int64(channel), s)
		if err != nil {
			fmt.Printf("%+v\n", err)
		}
	}
}

func SendPacket(payload []byte, portnum pb.PortNum, to int64, channel int64, s *streamer) error {
	var address int64
	if to == 0 {
		address = broadcastNum
	} else {
		address = to
	}

	// This constant is defined in Constants_DATA_PAYLOAD_LEN, but not in a friendly way to use
	if len(payload) > 240 {
		return errors.New("message too large")
	}

	rand.Seed(time.Now().UnixNano())
	packetID := rand.Intn(2386828-1) + 1

	radioMessage := pb.ToRadio{
		PayloadVariant: &pb.ToRadio_Packet{
			Packet: &pb.MeshPacket{
				To:      uint32(address),
				WantAck: true,
				Id:      uint32(packetID),
				Channel: uint32(channel),
				PayloadVariant: &pb.MeshPacket_Decoded{
					Decoded: &pb.Data{
						Payload: payload,
						Portnum: portnum,
						//Portnum: 257,
					},
				},
			},
		},
	}

	out, err := proto.Marshal(&radioMessage)
	if err != nil {
		return err
	}

	if err := sendPacket(out, s); err != nil {
		return err
	}

	return nil

}

func sendPacket(protobufPacket []byte, s *streamer) (err error) {

	packageLength := len(string(protobufPacket))

	header := []byte{start1, start2, byte(packageLength>>8) & 0xff, byte(packageLength) & 0xff}

	radioPacket := append(header, protobufPacket...)
	err = s.Write(radioPacket)
	if err != nil {
		return err
	}

	return

}
