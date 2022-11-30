package main

import (
	"container/heap"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Op struct {
	ID        string `json:"id"`
	Key       string `json:"key"`
	Value     int    `json:"value"`
	Peer      string `json:"peer"`
	Timestamp int64  `json:"timestamp"`
}

type Row struct {
	value     int
	peer      string
	timestamp int64
}

type edge struct {
	child   *Node
	parent  *Node
	counter int
}

type Observer func(op Op, origin string, oldValue int, ok bool)

type DB struct {
	peer      string
	rows      map[IdKey]Row
	observers []Observer
}

func (db *DB) afterApply(ob Observer) {
	db.observers = append(db.observers, ob)
}

type IdKey struct {
	ID, Key string
}

// func (db *DB) getIds() []string {
// 	var keys []string
// 	for k, _ := range db.rows {
// 		keys = append(keys, k)
// 	}
// 	return keys
// }

func (db *DB) getTimestamp(idkey IdKey) (int64, bool) {
	if r, e := db.rows[idkey]; e {
		return r.timestamp, true
	}
	return 0, false
}

func (db *DB) getValue(idkey IdKey) (int, bool) {
	if r, e := db.rows[idkey]; e {
		return r.value, true
	}
	return -1, false
}

func Max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func (db *DB) setValue(idkey IdKey, value int) {
	op := Op{
		ID:        idkey.ID,
		Key:       idkey.Key,
		Value:     value,
		Peer:      db.peer,
		Timestamp: time.Now().UnixNano(),
	}
	if timestamp, e := db.getTimestamp(idkey); e {
		op.Timestamp = Max(op.Timestamp, timestamp+1)
	}
	db.apply(op, "local")
}

func (db *DB) apply(op Op, origin string) {
	field, e := db.rows[IdKey{op.ID, op.Key}]
	if !(e && field.timestamp > op.Timestamp || (field.timestamp == op.Timestamp && field.peer > op.Peer)) {
		db.rows[IdKey{op.ID, op.Key}] = Row{
			value:     op.Value,
			peer:      op.Peer,
			timestamp: op.Timestamp,
		}
	}
	for _, observe := range db.observers {
		observe(op, origin, field.value, e)
	}
}

func newDB(id string) *DB {
	db := &DB{
		peer: id,
		rows: make(map[IdKey]Row),
	}
	return db
}

type Node struct {
	id       string
	parent   *Node
	children []*Node
	edges    map[string]int
	cycle    string // FIXME
}

func newNodeWithID(id string) *Node {
	return &Node{
		id:       id,
		parent:   nil,
		children: nil,
		edges:    make(map[string]int),
		cycle:    "",
	}
}

type Tree struct {
	db    *DB
	root  *Node
	nodes map[string]*Node
}

func newTree(db *DB) *Tree {
	tree := &Tree{
		db:    db,
		root:  newNodeWithID("(ROOT)"),
		nodes: make(map[string]*Node),
	}

	tree.nodes[tree.root.id] = tree.root

	db.afterApply(func(op Op, _ string, _ int, _ bool) {
		var child *Node
		child, e := tree.nodes[op.ID]
		if !e {
			child = newNodeWithID(op.ID)
			tree.nodes[op.ID] = child
		}
		if _, e := tree.nodes[op.Key]; !e {
			tree.nodes[op.Key] = newNodeWithID(op.Key)
		}

		if op.Value == -1 {
			delete(child.edges, op.Key)
		} else {
			child.edges[op.Key] = op.Value
		}

		tree.recomputeParentsAndChildren()
	})

	return tree
}

// The edge with the largest counter is considered to be the most recent
// one. If two edges are set simultaneously, the identifier breaks the tie.
func edgeWithLargestCounter(node *Node) string {
	var edgeID string
	largestCounter := -1
	for id, counter := range node.edges {
		if counter > largestCounter || (counter == largestCounter && id > edgeID) {
			edgeID = id
			largestCounter = counter
		}
	}
	return edgeID
}

// Returns true if and only if "node" is in the subtree under "other".
// This function is safe to call in the presence of parent cycles.
func isNodeUnderOtherNode(node *Node, other *Node) bool {
	if node == other {
		return true
	}
	tortoise := node
	hare := node.parent
	for hare != nil && hare != other {
		if tortoise == hare {
			// Cycle detected
			return false
		}
		hare = hare.parent
		if hare == nil || hare == other {
			break
		}
		tortoise = tortoise.parent
		hare = hare.parent
	}
	return hare == other
}

func (tree *Tree) recomputeParentsAndChildren() {
	for _, node := range tree.nodes {
		node.parent = tree.nodes[edgeWithLargestCounter(node)]
		node.children = nil
	}

	nonRootedNodes := make(map[*Node]bool)
	for _, node := range tree.nodes {
		if !isNodeUnderOtherNode(node, tree.root) {
			for node != nil {
				if !nonRootedNodes[node] {
					nonRootedNodes[node] = true
					node = node.parent
				} else {
					break
				}
			}
		}
	}

	if len(nonRootedNodes) > 0 {
		deferredEdges := make(map[string][]edge)
		readyEdges := &PriorityQueue{}

		for child := range nonRootedNodes {
			for parentID, counter := range child.edges {
				parent := tree.nodes[parentID]
				if _, e := nonRootedNodes[parent]; !e {
					heap.Push(readyEdges, &Item{
						value: edge{
							child:   child,
							parent:  parent,
							counter: counter,
						},
					})
				} else {
					edges, e := deferredEdges[parent.id]
					if !e {
						edges = nil
					}
					edges = append(edges, edge{
						child:   child,
						parent:  parent,
						counter: counter,
					})
					deferredEdges[parent.id] = edges
				}
			}
		}

		for len(*readyEdges) > 0 {
			top := heap.Pop(readyEdges).(*Item)
			child := top.value.child
			if _, e := nonRootedNodes[child]; !e {
				continue
			}

			child.parent = top.value.parent
			delete(nonRootedNodes, child)

			edges, e := deferredEdges[child.id]
			if e {
				for _, edge := range edges {
					heap.Push(readyEdges, &Item{
						value: edge,
					})
				}
			}
		}
	}
	for _, node := range tree.nodes {
		if node.parent != nil {
			node.parent.children = append(node.parent.children, node)
		}
	}

	for _, node := range tree.nodes {
		// FIXME: add different order for real lists
		sort.Slice(node.children, func(i, j int) bool {
			return node.children[i].id < node.children[j].id
		})
	}
}

type edit struct {
	child, parent *Node
}

func (tree *Tree) addChildToParent(childID, parentID string) {
	var edits []edit

	ensureNodeIsRooted := func(child *Node) {
		for child != nil {
			parent := child.parent
			if parent == nil {
				break
			}
			edge := edgeWithLargestCounter(child)
			if edge != parent.id {
				edits = append(edits, edit{
					child:  child,
					parent: parent,
				})
			}
			child = parent
		}
	}

	child := tree.nodes[childID]
	parent := tree.nodes[parentID]
	ensureNodeIsRooted(child.parent)
	ensureNodeIsRooted(parent)
	edits = append(edits, edit{
		child:  child,
		parent: parent,
	})

	for _, edit := range edits {
		maxCounter := -1

		child := edit.child
		parent := edit.parent

		for _, counter := range child.edges {
			if maxCounter < counter {
				maxCounter = counter
			}
		}

		tree.db.setValue(IdKey{child.id, parent.id}, maxCounter+1)
	}
}

// func main() {
// db := newDB("asdfasdf")
// tree := newTree(db)
// db.setValue(IdKey{"a", "b"}, 0)
// db.setValue(IdKey{"b", tree.root.ID}, 0)
// db.setValue(IdKey{"c", tree.root.ID}, 0)
// db.setValue(IdKey{"d", "c"}, -2)
//
// db.afterApply(func(op Op, origin string, oldValue int, ok bool) {
// 	// show(tree.root, 0)
// })
//
// show(tree.root, 0)
//
// rl := readline.NewInstance()
// for {
// 	line, err := rl.Readline()
// 	if err != nil {
// 		fmt.Println("error:", err)
// 		return
// 	}
//
// 	parts := strings.Split(line, " ")
// 	childID := strings.TrimSpace(parts[0])
// 	parentID := strings.TrimSpace(parts[1])
// 	fmt.Printf("[%s][%s]\n", childID, parentID)
// 	if _, e := tree.nodes[childID]; !e {
// 		tree.nodes[childID] = newNodeWithID(childID)
// 	}
//
// 	if _, e := tree.nodes[parentID]; e {
// 		tree.addChildToParent(childID, parentID)
// 	} else {
// 		fmt.Println("unknown parent", parentID)
// 	}
// 	show(tree.root, 0)
// }
// }

func show(it *Node, indent int) {
	if it == nil {
		return
	}
	fmt.Printf("%s%s\n", strings.Repeat(" ", indent), it.id)
	for _, c := range it.children {
		show(c, indent+2)
	}
}
