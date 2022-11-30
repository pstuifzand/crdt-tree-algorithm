import {Tree as CRDTTree} from './tree'
import DB from './db'
import {Tree as YYTree} from 'yy-tree'

const db = new DB("web");
const tree = new CRDTTree(db);
const yytree = new YYTree(tree.root, {parent: document.body})

yytree.on('move', (target, lyytree) => {
    console.log(target.data.id, target.data.parent.id)
    tree.addChildToParent(target.data.id, target.data.parent.id)
})
yytree.on('name-change', (leaf, name, lyytree) => {
    console.log(leaf, leaf.data.name)
    const n = tree.nodes.get(leaf.data.id)
    n.name = leaf.data.name
})

function visit(node, indent) {
    console.log("--".repeat(indent), node.id)
    for (const child of node.children) {
        visit(child, indent + 2)
    }
}

const ws = new WebSocket("ws://127.0.0.1:12345/sub", "pub.sp.nanomsg.org")
ws.onmessage = function (ev) {
    ev.data.text()
        .then(data => JSON.parse(data))
        .then(op => db.apply(op, 'remote'))
        .then(() => visit(tree.root, 0))
        .then(() => yytree.update())
}
