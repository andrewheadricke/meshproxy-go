package main

import (
	//"io"
	//"bytes"
	//"fmt"
	"net"
	"bufio"
	//"errors"
	"net/http"
)

type CustomResponseWriter struct {
	statusCode  int
	header      http.Header
	conn        net.Conn
	headersSent bool
}

func NewCustomResponseWriter(conn net.Conn) *CustomResponseWriter {
	return &CustomResponseWriter{
		header: http.Header{},
		statusCode: http.StatusOK,
		conn: conn,
	}
}

func (w *CustomResponseWriter) Header() http.Header {
	return w.header
}

func (w *CustomResponseWriter) Write(b []byte) (int, error) {
	//fmt.Printf("Write->headers %+v\n", w.header)
	if !w.headersSent {
		for key, value := range w.header {
			w.conn.Write([]byte(key + ": " + value[0] + "\n"))
		}
		w.conn.Write([]byte("\n"))
		w.headersSent = true
	}
	
	w.conn.Write(b)
	return len(b), nil
}

func (w *CustomResponseWriter) WriteHeader(statusCode int) {
	//fmt.Printf("writeheader %+v\n", statusCode)
	if statusCode == 200 {
		w.conn.Write([]byte("HTTP/1.1 200 OK\n"))
	} else if statusCode == 404 {
		w.conn.Write([]byte("HTTP/1.1 404 Not Found\n"))
	}
}

func (w *CustomResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	rw := bufio.NewReadWriter(bufio.NewReader(w.conn), bufio.NewWriter(w.conn))
	return w.conn, rw, nil
}
