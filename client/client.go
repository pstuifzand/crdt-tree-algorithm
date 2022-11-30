package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/fossoreslp/go-uuid-v4"
	"github.com/lmorg/readline"
	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/protocol/push"
	"go.nanomsg.org/mangos/v3/protocol/sub"

	// register ws transport
	_ "go.nanomsg.org/mangos/v3/transport/ws"
)

func die(format string, v ...interface{}) {
	fmt.Fprintln(os.Stderr, fmt.Sprintf(format, v...))
	os.Exit(1)
}

func pullClient(port int, c chan Op) {
	sock, err := push.NewSocket()
	if err != nil {
		die("cannot make pull socket: %v", err)
	}

	url := fmt.Sprintf("ws://127.0.0.1:%d/push", port)
	if err = sock.Dial(url); err != nil {
		die("cannot dial req url: %v", err)
	}
	go func() {
		for op := range c {
			msg, err := json.Marshal(op)
			if err != nil {
				die("cannot marshal message", op)
			}
			if err := sock.Send(msg); err != nil {
				die("Cannot send push: %v", err)
			}
		}
	}()
}

// subClient implements the client for SUB.
func subClient(port int, c chan Op) {
	sock, err := sub.NewSocket()
	if err != nil {
		die("cannot make req socket: %v", err)
	}
	if err = sock.SetOption(mangos.OptionSubscribe, []byte{}); err != nil {
		die("cannot set subscription: %v", err)
	}
	url := fmt.Sprintf("ws://127.0.0.1:%d/sub", port)
	if err = sock.Dial(url); err != nil {
		die("cannot dial req url: %v", err)
	}
	go func() {
		for {
			if m, err := sock.Recv(); err != nil {
				die("Cannot recv sub: %v", err)
			} else {
				// convert to Op and send on channel
				var op Op
				err = json.Unmarshal(m, &op)
				if err != nil {
					log.Fatal("unmarshal in sub", err)
				}
				c <- op
				spew.Dump(op)
			}
		}
	}()
}

func main() {
	sendC := make(chan Op)
	recvC := make(chan Op)

	port := 12345
	subClient(port, recvC)
	pullClient(port, sendC)

	id, _ := uuid.NewString()

	db := newDB(id)
	tree := newTree(db)

	// aid, _ := uuid.NewString()
	// bid, _ := uuid.NewString()
	// cid, _ := uuid.NewString()
	// did, _ := uuid.NewString()

	db.afterApply(func(op Op, origin string, oldValue int, ok bool) {
		if origin == "local" {
			sendC <- op
		}
	})

	// db.setValue(IdKey{aid, bid}, 0)
	// db.setValue(IdKey{bid, tree.root.ID}, 0)
	// db.setValue(IdKey{cid, tree.root.ID}, 0)
	// db.setValue(IdKey{did, cid}, -2)

	// db.afterApply(func(op Op, origin string, oldValue int, ok bool) {
	// 	// show(tree.root, 0)
	// })

	show(tree.root, 0)

	go func() {
		for op := range recvC {
			log.Println(op)
			db.apply(op, "remote")
		}
	}()

	mapping := make(map[string]string)
	mapping[tree.root.id] = tree.root.id

	rl := readline.NewInstance()
	for {
		line, err := rl.Readline()
		if err != nil {
			fmt.Println("error:", err)
			return
		}

		parts := strings.Split(line, " ")
		if len(parts) != 2 {
			show(tree.root, 0)
			continue
		}

		childName := strings.TrimSpace(parts[0])
		parentName := strings.TrimSpace(parts[1])

		// Create ID's
		childID, e := mapping[childName]
		if !e {
			id, _ := uuid.NewString()
			mapping[childName] = id
			childID = id
		}
		parentID, e := mapping[parentName]
		if !e {
			id, _ := uuid.NewString()
			mapping[parentName] = id
			parentID = id
		}

		fmt.Printf("[%s][%s]\n", childID, parentID)
		if _, e := tree.nodes[childID]; !e {
			tree.nodes[childID] = newNodeWithID(childID, childName)
		}

		if _, e := tree.nodes[parentID]; e {
			tree.addChildToParent(childID, parentID)
		} else {
			fmt.Println("unknown parent", parentID)
		}
		show(tree.root, 0)
	}
}
