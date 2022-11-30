// This is a simple last-writer-wins peer-to-peer database for use with
// demos. It's in this separate file because all demos share this code.
class DB {
    constructor(peer) {
        this._peer = peer
        this._rows = new Map
        this._observers = []
    }

    afterApply(observer) {
        this._observers.push(observer)
    }

    get ids() {
        return [...this._rows.keys()]
    }

    get(id, key) {
        const row = this._rows.get(id)
        const field = row && row.get(key)
        return field && field.value
    }

    set(id, key, value) {
        const op = { id, key, value, peer: this._peer, timestamp: Date.now() }
        const row = this._rows.get(id)
        const field = row && row.get(key)
        if (field) {
            // Make sure "set" always overwrites locally.
            op.timestamp = Math.max(op.timestamp, field.timestamp + 1)
        }
        this.apply(op, 'local');
    }

    apply(op, origin) {
        let row = this._rows.get(op.id)
        if (!row) {
            row = new Map
            this._rows.set(op.id, row)
        }
        const field = row.get(op.key)
        if (field && (field.timestamp > op.timestamp || (field.timestamp === op.timestamp && field.peer > op.peer))) {
            // Don't overwrite newer values with older values. The last writer always wins.
        } else {
            row.set(op.key, { peer: op.peer, timestamp: op.timestamp, value: op.value })
        }
        for (const observer of this._observers) {
            observer({ op, origin, oldValue: field && field.value })
        }
    }
}

class UndoRedo {
    constructor(db, { onlyKeys } = {}) {
        this.db = db
        this.undoHistory = []
        this.redoHistory = []
        this._onlyKeys = onlyKeys && new Set(onlyKeys)
        this._isBusy = false
        this._pending = []
        this._depth = 0

        db.afterApply(({ op, origin, oldValue }) => {
            if (origin === 'local' && !this._isBusy && (!this._onlyKeys || this._onlyKeys.has(op.key))) {
                this._pending.push({ id: op.id, key: op.key, value: oldValue })
                this._commit()
            }
        })
    }

    batch(callback) {
        this._depth++
        callback()
        this._depth--
        this._commit()
    }

    undo() {
        const top = this.undoHistory.pop()
        if (top) this.redoHistory.push(this._apply(top))
    }

    redo() {
        const top = this.redoHistory.pop()
        if (top) this.undoHistory.push(this._apply(top))
    }

    _commit() {
        if (this._depth === 0) {
            this.undoHistory.push(this._pending)
            this.redoHistory = []
            this._pending = []
        }
    }

    _apply(changes) {
        const reverse = []
        this._isBusy = true
        for (const { id, key, value } of changes) {
            reverse.push({ id, key, value: this.db.get(id, key) })
            this.db.set(id, key, value)
        }
        this._isBusy = false
        return reverse.reverse()
    }
}


class Tree {
    constructor(db) {
        const newNodeWithID = id => ({id, parent: null, children: [], edges: new Map, cycle: null})
        this.db = db
        this.root = newNodeWithID('(ROOT)')
        this.nodes = new Map
        this.nodes.set(this.root.id, this.root)

        // Keep the in-memory tree up to date and cycle-free as the database is mutated
        db.afterApply(({op}) => {
            // Each mutation takes place on the child. The key is the parent
            // identifier and the value is the counter for that graph edge.
            let child = this.nodes.get(op.id)
            if (!child) {
                // Make sure the child exists
                child = newNodeWithID(op.id)
                this.nodes.set(op.id, child)
            }
            if (!this.nodes.has(op.key)) {
                // Make sure the parent exists
                this.nodes.set(op.key, newNodeWithID(op.key))
            }
            if (op.value === undefined) {
                // Undo can revert a value back to undefined
                child.edges.delete(op.key)
            } else {
                // Otherwise, add an edge from the child to the parent
                child.edges.set(op.key, op.value)
            }
            this.recomputeParentsAndChildren()
        })
    }

    recomputeParentsAndChildren() {
        // Start off with all children arrays empty and each parent pointer
        // for a given node set to the most recent edge for that node.
        for (const node of this.nodes.values()) {
            // Set the parent identifier to the link with the largest counter
            node.parent = this.nodes.get(edgeWithLargestCounter(node)) || null
            node.children = []
        }

        // At this point all nodes that can reach the root form a tree (by
        // construction, since each node other than the root has a single
        // parent). The parent pointers for the remaining nodes may form one
        // or more cycles. Gather all remaining nodes detached from the root.
        const nonRootedNodes = new Set
        for (let node of this.nodes.values()) {
            if (!isNodeUnderOtherNode(node, this.root)) {
                while (node && !nonRootedNodes.has(node)) {
                    nonRootedNodes.add(node)
                    node = node.parent
                }
            }
        }

        // Deterministically reattach these nodes to the tree under the root
        // node. The order of reattachment is arbitrary but needs to be based
        // only on information in the database so that all peers reattach
        // non-rooted nodes in the same order and end up with the same tree.
        if (nonRootedNodes.size > 0) {
            // All "ready" edges already have the parent connected to the root,
            // and all "deferred" edges have a parent not yet connected to the
            // root. Prioritize newer edges over older edges using the counter.
            const deferredEdges = new Map
            const readyEdges = new PriorityQueue((a, b) => {
                const counterDelta = b.counter - a.counter
                if (counterDelta !== 0) return counterDelta
                if (a.parent.id < b.parent.id) return -1
                if (a.parent.id > b.parent.id) return 1
                if (a.child.id < b.child.id) return -1
                if (a.child.id > b.child.id) return 1
                return 0
            })
            for (const child of nonRootedNodes) {
                for (const [parentID, counter] of child.edges) {
                    const parent = this.nodes.get(parentID)
                    if (!nonRootedNodes.has(parent)) {
                        readyEdges.push({child, parent, counter})
                    } else {
                        let edges = deferredEdges.get(parent)
                        if (!edges) {
                            edges = []
                            deferredEdges.set(parent, edges)
                        }
                        edges.push({child, parent, counter})
                    }
                }
            }
            for (let top; top = readyEdges.pop();) {
                // Skip nodes that have already been reattached
                const child = top.child
                if (!nonRootedNodes.has(child)) continue

                // Reattach this node
                child.parent = top.parent
                nonRootedNodes.delete(child)

                // Activate all deferred edges for this node
                const edges = deferredEdges.get(child)
                if (edges) for (const edge of edges) readyEdges.push(edge)
            }
        }

        // Add items as children of their parents so that the rest of the app
        // can easily traverse down the tree for drawing and hit-testing
        for (const node of this.nodes.values()) {
            if (node.parent) {
                node.parent.children.push(node)
            }
        }

        // Sort each node's children by their identifiers so that all peers
        // display the same tree. In this demo, the ordering of siblings
        // under the same parent is considered unimportant. If this is
        // important for your app, you will need to use another CRDT in
        // combination with this CRDT to handle the ordering of siblings.
        for (const node of this.nodes.values()) {
            node.children.sort((a, b) => {
                if (a.id < b.id) return -1
                if (a.id > b.id) return 1
                return 0
            })
        }
    }

    addChildToParent(childID, parentID) {
        const ensureNodeIsRooted = child => {
            while (child) {
                const parent = child.parent
                if (!parent) break
                const edge = edgeWithLargestCounter(child)
                if (edge !== parent.id) edits.push([child, parent])
                child = parent
            }
        }

        // Ensure that both the old and new parents remain where they are
        // in the tree after the edit we are about to make. Then move the
        // child from its old parent to its new parent.
        const edits = []
        const child = this.nodes.get(childID)
        const parent = this.nodes.get(parentID)
        ensureNodeIsRooted(child.parent)
        ensureNodeIsRooted(parent)
        edits.push([child, parent])

        // Apply all database edits accumulated above. If your database
        // supports syncing a set of changes in a single batch, then these
        // edits should all be part of the same batch for efficiency. The
        // order that these edits are made in shouldn't matter.
        for (const [child, parent] of edits) {
            let maxCounter = -1
            for (const counter of child.edges.values()) {
                maxCounter = Math.max(maxCounter, counter)
            }
            this.db.set(child.id, parent.id, maxCounter + 1)
        }
    }
}

// Note: This priority queue implementation is inefficient. It should
// probably be implemented using a heap instead. This only matters when
// there area large numbers of edges on nodes involved in cycles.
class PriorityQueue {
    constructor(compare) {
        this.compare = compare
        this.items = []
    }

    push(item) {
        this.items.push(item)
        this.items.sort(this.compare)
    }

    pop() {
        return this.items.shift()
    }
}

// The edge with the largest counter is considered to be the most recent
// one. If two edges are set simultaneously, the identifier breaks the tie.
function edgeWithLargestCounter(node) {
    let edgeID = null
    let largestCounter = -1
    for (const [id, counter] of node.edges) {
        if (counter > largestCounter || (counter === largestCounter && id > edgeID)) {
            edgeID = id
            largestCounter = counter
        }
    }
    return edgeID
}

// Returns true if and only if "node" is in the subtree under "other".
// This function is safe to call in the presence of parent cycles.
function isNodeUnderOtherNode(node, other) {
    if (node === other) return true
    let tortoise = node
    let hare = node.parent
    while (hare && hare !== other) {
        if (tortoise === hare) return false // Cycle detected
        hare = hare.parent
        if (!hare || hare === other) break
        tortoise = tortoise.parent
        hare = hare.parent
    }
    return hare === other
}