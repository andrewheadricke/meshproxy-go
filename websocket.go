package main

import (
	"fmt"
	"log"	
	"bytes"
	"bufio"
	"net/http"
	"encoding/json"

	"github.com/gorilla/websocket"
	//"google.golang.org/protobuf/proto"
  //pb "example.com/meshproxy-go/gomeshproto"
)

var chanWebsocketComplete chan bool

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
 },
}

func CheckForWebsocketUpgrade(buf []byte, clientConn *ClientConnection, s *streamer) chan bool {
	//fmt.Printf("%+v %+v\n", buf, string(buf))

	if bytes.Equal(buf[:16], []byte("GET / HTTP/1.1\r\n")) {
		chanWebsocketComplete := make(chan bool)

		fmt.Printf("http request detected\n")

		req := getHttpRequest(buf)
		rw := NewCustomResponseWriter(clientConn.rawConn)

		wsConn, err := upgrader.Upgrade(rw, req, nil)	
		if err != nil {
				log.Println(err)
				return nil
		}

		//fmt.Printf("%+v\n", wsConn)
		clientConn.webSocket = wsConn

		go func() {
			for {
				_, data, err := wsConn.ReadMessage()
				if err != nil {
					wsConn.Close()
					chanWebsocketComplete <- true
					return
				}
				EncodeProtobufAndSend(data, s, clientConn)
			}
		}()

		return chanWebsocketComplete
	}

	return nil
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
	}
}