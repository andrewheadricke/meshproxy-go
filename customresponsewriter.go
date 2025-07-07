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
	statusCode int
	header     http.Header
	conn       net.Conn
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
	//fmt.Printf("write\n")
	return 0, nil
}

func (w *CustomResponseWriter) WriteHeader(statusCode int) {
	//fmt.Printf("writeheader %+v\n", statusCode)
	w.statusCode = statusCode
}

func (w *CustomResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	rw := bufio.NewReadWriter(bufio.NewReader(w.conn), bufio.NewWriter(w.conn))
	return w.conn, rw, nil
}
