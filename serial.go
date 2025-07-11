
package main

import (
  "os"
  "io"
  "fmt"
  "time"
  "bytes"

  "go.bug.st/serial"
)

const start1 = byte(0x94)
const start2 = byte(0xc3)

type streamer struct {
  address string
  serialPort io.ReadWriteCloser
  isConnected bool
  dontReconnect bool
}

func (s *streamer) Init(addr string) error {

  s.address = addr

  mode := &serial.Mode{
    BaudRate: 115200,
    Parity: serial.NoParity,
    DataBits: 8,
    StopBits: serial.OneStopBit,
  }

  port, err := serial.Open(addr, mode)
  if err != nil {
    fmt.Printf("%+v\n", err)
    return err
  }

  s.isConnected = true
  s.serialPort = port

  return nil
}

func (s *streamer) Reconnect() error {
  if s.address == "/dev/ttyUSB0" {
    s.address = "/dev/ttyUSB1" 
  } else if s.address == "/dev/ttyUSB1" {
    s.address = "/dev/ttyUSB2"
  } else if s.address == "/dev/ttyUSB2" {
    s.address = "/dev/ttyUSB3"
  } else if s.address == "/dev/ttyUSB3" {
    s.address = "/dev/ttyUSB0"
  } else if s.address == "/dev/ttyACM0" {
    s.address = "/dev/ttyACM1" 
  } else if s.address == "/dev/ttyACM1" {
    s.address = "/dev/ttyACM2"
  } else if s.address == "/dev/ttyACM2" {
    s.address = "/dev/ttyACM3"
  } else if s.address == "/dev/ttyACM3" {
    s.address = "/dev/ttyACM0"
  }

  mode := &serial.Mode{
    BaudRate: 115200,
    Parity: serial.NoParity,
    DataBits: 8,
    StopBits: serial.OneStopBit,
  }

  port, err := serial.Open(s.address, mode)
  if err != nil {
    fmt.Printf("%+v\n", err)
    time.Sleep(1 * time.Second)
    return err
  }

  s.isConnected = true
  s.serialPort = port
  return nil
}


func (s *streamer) Close() {
  s.dontReconnect = true
  s.serialPort.Close()
  s.isConnected = false
}

func (s *streamer) Read(p []byte) (int, error) {

  size, err := s.serialPort.Read(p)
  if err != nil {
    return 0, err
  }

  //fmt.Printf("Size: %+v\n", size)

  return size, nil
}

func (s *streamer) Write(p []byte) error {

  _, err := s.serialPort.Write(p)
  if err != nil {
    return err
  }

  time.Sleep(100 * time.Millisecond)

  return nil
}

/*
func IsPortClosedError(err error) bool {
  sErr, _ := err.(serial.PortError)
  fmt.Printf("%+v\n", sErr)
  if sErr.Code() == serial.PortClosed {
    return true
  } else {
    return false
  }
}
*/

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