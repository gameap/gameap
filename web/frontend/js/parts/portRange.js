// A node's pool of ports for new servers, kept in its metadata as
// comma-separated ports and inclusive ranges: "27015-27100, 28000". The rules
// mirror domain.ParsePortRange in the API.
export const PORT_RANGE_KEY = 'port_range'

const MIN_PORT = 1
const MAX_PORT = 65535
const ENTRY = /^\s*(\d+)\s*(?:-\s*(\d+)\s*)?$/

// parsePortRange returns the pool as sorted, merged [{from, to}] spans, an
// empty list for blank text and null when the text is not a valid pool.
export function parsePortRange(text) {
    if (typeof text !== 'string' || text.trim() === '') {
        return []
    }

    const spans = []
    for (const entry of text.split(',')) {
        const match = ENTRY.exec(entry)
        if (!match) {
            return null
        }

        const from = Number(match[1])
        const to = match[2] === undefined ? from : Number(match[2])
        if (from < MIN_PORT || to > MAX_PORT || from > to) {
            return null
        }

        spans.push({from, to})
    }

    spans.sort((a, b) => a.from - b.from)

    return spans.reduce((merged, span) => {
        const last = merged[merged.length - 1]
        if (last && span.from <= last.to + 1) {
            last.to = Math.max(last.to, span.to)
        } else {
            merged.push({...span})
        }

        return merged
    }, [])
}

export function inPortRange(spans, port) {
    return spans.some(({from, to}) => port >= from && port <= to)
}

// findFreePort returns the first port isFree accepts. With a pool it walks the
// pool from start upwards and then from the beginning of the pool; without one
// it counts up from start. null when no port fits.
export function findFreePort(spans, start, isFree) {
    for (const port of candidates(spans, start)) {
        if (isFree(port)) {
            return port
        }
    }

    return null
}

function* candidates(spans, start) {
    if (spans.length === 0) {
        for (let port = start; port <= MAX_PORT; port++) {
            yield port
        }

        return
    }

    for (const {from, to} of spans) {
        for (let port = Math.max(from, start); port <= to; port++) {
            yield port
        }
    }

    for (const {from, to} of spans) {
        for (let port = from; port <= Math.min(to, start - 1); port++) {
            yield port
        }
    }
}
