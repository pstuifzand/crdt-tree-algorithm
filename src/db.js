export default class DB {
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
        const op = {id, key, value, peer: this._peer, timestamp: Date.now()}
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
            row.set(op.key, {peer: op.peer, timestamp: op.timestamp, value: op.value, name:op.id})
        }
        for (const observer of this._observers) {
            observer({op, origin, oldValue: field && field.value})
        }
    }
}
