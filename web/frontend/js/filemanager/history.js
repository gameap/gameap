/**
 * Visit/open history for the file manager: which directories the user works
 * in and which files they open, ranked by recency or by a decaying frequency
 * score. Pure functions over a plain state object — useHistoryStore owns
 * storage, timers and the current wall clock, so everything here takes `now`
 * as an argument.
 *
 * State shape (one object per server+disk storage key):
 *   { version: 1, view: 'recent' | 'frequent',
 *     entries: { dir: { [path]: entry }, file: { [path]: entry } } }
 *   entry = { count, lastAt, score, scoreAt }
 *
 * Frequency uses exponential decay: every visit adds 1 to the score, and the
 * accumulated score halves every HALF_LIFE_MS, so old favourites gradually
 * give way to new ones instead of freezing the "frequent" list forever.
 *
 * Paths are in the file manager's native format: disk-relative, no leading
 * slash ('cstrike/server.cfg'); '' is the disk root and is never recorded.
 */

export const HISTORY_VERSION = 1
export const HALF_LIFE_MS = 7 * 24 * 60 * 60 * 1000
export const RECENT_WINDOW_MS = 30 * 24 * 60 * 60 * 1000
export const MAX_ENTRIES_PER_KIND = 200
export const DWELL_MS = 5000
export const TOP_LIMIT = 4

const KINDS = ['dir', 'file']
const VIEWS = ['recent', 'frequent']

export function storageKey(serverId, disk) {
    return `gameap:fm:history:${serverId}:${disk}`
}

export function createEmptyState() {
    return {
        version: HISTORY_VERSION,
        view: 'recent',
        entries: { dir: {}, file: {} },
    }
}

/**
 * Parse and validate a raw storage string. Corrupt JSON, a foreign version or
 * malformed entries all fall back to an empty state — history is a
 * convenience, never worth an error.
 */
export function loadState(raw) {
    if (!raw) return createEmptyState()
    try {
        return normalizeState(JSON.parse(raw))
    } catch (e) {
        return createEmptyState()
    }
}

export function normalizeState(data) {
    if (!data || data.version !== HISTORY_VERSION || !data.entries || typeof data.entries !== 'object') {
        return createEmptyState()
    }

    const state = createEmptyState()
    if (VIEWS.includes(data.view)) {
        state.view = data.view
    }

    for (const kind of KINDS) {
        const bucket = data.entries[kind]
        if (!bucket || typeof bucket !== 'object') continue

        for (const [path, entry] of Object.entries(bucket)) {
            if (!path || !entry || typeof entry.lastAt !== 'number') continue

            state.entries[kind][path] = {
                count: typeof entry.count === 'number' ? entry.count : 1,
                lastAt: entry.lastAt,
                score: typeof entry.score === 'number' ? entry.score : 1,
                scoreAt: typeof entry.scoreAt === 'number' ? entry.scoreAt : entry.lastAt,
            }
        }
    }

    return state
}

// Negative elapsed is clamped so a system clock set back does not inflate
// scores recorded "in the future".
export function decayedScore(entry, now) {
    return entry.score * 0.5 ** (Math.max(0, now - entry.scoreAt) / HALF_LIFE_MS)
}

export function recordVisit(state, kind, path, now) {
    const bucket = state.entries[kind]
    const prev = bucket[path]

    bucket[path] = {
        count: (prev ? prev.count : 0) + 1,
        lastAt: now,
        score: (prev ? decayedScore(prev, now) : 0) + 1,
        scoreAt: now,
    }

    pruneEntries(state, kind, now)
}

export function pruneEntries(state, kind, now, max = MAX_ENTRIES_PER_KIND) {
    const bucket = state.entries[kind]
    const paths = Object.keys(bucket)
    if (paths.length <= max) return

    paths
        .sort((a, b) => decayedScore(bucket[a], now) - decayedScore(bucket[b], now))
        .slice(0, paths.length - max)
        .forEach((path) => {
            delete bucket[path]
        })
}

export function removeDeleted(state, { path, type }) {
    if (type === 'dir') {
        const prefix = `${path}/`
        for (const kind of KINDS) {
            const bucket = state.entries[kind]
            for (const p of Object.keys(bucket)) {
                if (p === path || p.startsWith(prefix)) {
                    delete bucket[p]
                }
            }
        }
    } else {
        delete state.entries.file[path]
    }
}

export function moveRenamed(state, { type, oldPath, newPath }) {
    if (oldPath === newPath) return

    if (type === 'dir') {
        const prefix = `${oldPath}/`
        for (const kind of KINDS) {
            const bucket = state.entries[kind]
            for (const p of Object.keys(bucket)) {
                if (p === oldPath || p.startsWith(prefix)) {
                    bucket[newPath + p.slice(oldPath.length)] = bucket[p]
                    delete bucket[p]
                }
            }
        }
    } else if (state.entries.file[oldPath]) {
        state.entries.file[newPath] = state.entries.file[oldPath]
        delete state.entries.file[oldPath]
    }
}

export function dropEntry(state, kind, path) {
    delete state.entries[kind][path]
}

export function topEntries(state, kind, view, now, limit = TOP_LIMIT) {
    const bucket = state.entries[kind]
    let items = Object.keys(bucket).map((path) => {
        const entry = bucket[path]
        const slash = path.lastIndexOf('/')

        return {
            path,
            basename: slash === -1 ? path : path.slice(slash + 1),
            dirname: slash === -1 ? '' : path.slice(0, slash),
            count: entry.count,
            lastAt: entry.lastAt,
            score: decayedScore(entry, now),
        }
    })

    if (view === 'recent') {
        items = items.filter((item) => now - item.lastAt < RECENT_WINDOW_MS)
        items.sort((a, b) => b.lastAt - a.lastAt)
    } else {
        items.sort((a, b) => b.score - a.score || b.lastAt - a.lastAt)
    }

    return items.slice(0, limit)
}

/**
 * Compact relative time for the "recent" view. Unit labels come from the
 * lang file ({n} placeholder, no plural forms needed); anything older than a
 * week falls back to a locale-formatted date.
 */
export function formatRelativeTime(lastAt, now, labels, localeDateFn) {
    const minutes = Math.floor(Math.max(0, now - lastAt) / 60000)
    if (minutes < 1) return labels.justNow
    if (minutes < 60) return labels.min.replace('{n}', String(minutes))

    const hours = Math.floor(minutes / 60)
    if (hours < 24) return labels.hour.replace('{n}', String(hours))
    if (hours < 48) return labels.yesterday

    const days = Math.floor(hours / 24)
    if (days < 7) return labels.day.replace('{n}', String(days))

    return localeDateFn(lastAt)
}

/**
 * localStorage wrapper: every operation is guarded, and after the first
 * failure (private mode, quota, disabled storage) the store flips to an
 * in-memory Map for the rest of the session. The full state is rewritten on
 * every persist, so nothing recorded in-session is lost by the flip.
 */
export function createStorage() {
    let memory = null

    function fallback(error) {
        if (!memory) {
            console.error('File manager history: localStorage unavailable, keeping history in memory:', error)
            memory = new Map()
        }
        return memory
    }

    return {
        get(key) {
            if (memory) return memory.has(key) ? memory.get(key) : null
            try {
                return localStorage.getItem(key)
            } catch (e) {
                fallback(e)
                return null
            }
        },
        set(key, value) {
            if (memory) {
                memory.set(key, value)
                return
            }
            try {
                localStorage.setItem(key, value)
            } catch (e) {
                fallback(e).set(key, value)
            }
        },
    }
}
