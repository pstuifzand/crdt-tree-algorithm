package main

import (
	"fmt"
	"net/http"
	"time"

	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/protocol"
	"go.nanomsg.org/mangos/v3/protocol/pub"
	// register ws transport
	"go.nanomsg.org/mangos/v3/transport/ws"
)

// subHandler just spins on the socket and publishes messages.  It sends
// "PUB #<count> <time>".  Not very interesting...

func subHandler(sock mangos.Socket) {
	count := 0
	for {
		msg := fmt.Sprintf("PUB #%d %s", count, time.Now().String())
		if e := sock.Send([]byte(msg)); e != nil {
			die("Cannot send pub: %v", e)
		}
		time.Sleep(5 * time.Second)
		count++
	}
}

func addSubHandler(mux *http.ServeMux, port int) protocol.Socket {
	sock, _ := pub.NewSocket()

	url := fmt.Sprintf("ws://127.0.0.1:%d/sub", port)

	options := make(map[string]interface{})
	options[ws.OptionWebSocketCheckOrigin] = false
	if l, e := sock.NewListener(url, options); e != nil {
		die("bad listener: %v", e)
	} else if h, e := l.GetOption(ws.OptionWebSocketHandler); e != nil {
		die("cannot get handler: %v", e)
	} else {
		mux.Handle("/sub", h.(http.Handler))
		l.Listen()
	}

	// go subHandler(sock)
	return sock
}
