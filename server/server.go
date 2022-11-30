package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/davecgh/go-spew/spew"
)

// This example demonstrates a trivial echo server.
func main() {

	server(12345)
}

func die(format string, v ...interface{}) {
	fmt.Fprintln(os.Stderr, fmt.Sprintf(format, v...))
	os.Exit(1)
}

func server(port int) {
	mux := http.NewServeMux()

	sub := addSubHandler(mux, port)
	pull := addPullHandler(mux, port)

	go func() {
		for {
			m, _ := pull.Recv()
			spew.Dump(m)
			_ = sub.Send(m)
		}
	}()

	mux.Handle("/website/", http.FileServer(http.Dir("../static")))

	e := http.ListenAndServe(fmt.Sprintf(":%d", port), mux)
	die("Http server died: %v", e)
}
