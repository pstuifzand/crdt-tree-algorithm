# CRDT tree


Use code from [tree][tree] and [madebyevan][madebyevan] to create an updating
tree. Uses mangos to send messages from client to server to client.

This also contains a rewrite of the crdt in Go. Only the client uses this Go
version. The server just pushes messages to other clients.

[tree]: https://github.com/davidfig/tree
[madebyevan]: https://madebyevan.com/algos/crdt-mutable-tree-hierarchy/
