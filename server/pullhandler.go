package main

import (
	"fmt"
	"net/http"

	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/protocol"
	"go.nanomsg.org/mangos/v3/protocol/pull"
	"go.nanomsg.org/mangos/v3/transport/ws"
)

func addPullHandler(mux *http.ServeMux, port int) protocol.Socket {
	sock, _ := pull.NewSocket()

	url := fmt.Sprintf("ws://127.0.0.1:%d/push", port)

	if l, e := sock.NewListener(url, nil); e != nil {
		die("bad listener: %v", e)
	} else if h, e := l.GetOption(ws.OptionWebSocketHandler); e != nil {
		die("cannot get handler: %v", e)
	} else {
		mux.Handle("/push", h.(http.Handler))
		l.Listen()
	}

	// go pullHandler(sock)

	return sock
}

func pullHandler(sock mangos.Socket) {
	for {
		msg, e := sock.Recv()
		if e != nil {
			die("Cannot get request: %v", e)
		}
		fmt.Println(string(msg))
	}
}
