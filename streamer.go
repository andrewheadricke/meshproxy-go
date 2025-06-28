
package main

import (
	"io"
	"fmt"
	"time"

	"go.bug.st/serial"
)

type streamer struct {
	address string
	serialPort io.ReadWriteCloser
	isConnected bool
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
		s.address = "/dev/ttyUSB0"
	} else if s.address == "/dev/ttyACM0" {
		s.address = "/dev/ttyACM1" 
	} else if s.address == "/dev/ttyACM1" {
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

/*
func (s *streamer) Write(p []byte) error {

	_, err := s.serialPort.Write(p)
	if err != nil {
		return err
	}

	time.Sleep(100 * time.Millisecond)

	return nil
}

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