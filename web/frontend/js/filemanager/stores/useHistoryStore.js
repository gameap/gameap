import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { useSettingsStore } from './useSettingsStore.js'
import {
    DWELL_MS,
    createEmptyState,
    createStorage,
    dropEntry,
    loadState,
    moveRenamed,
    recordVisit,
    removeDeleted,
    storageKey,
    topEntries,
} from '../history.js'

/**
 * Visit/open history, persisted per server+disk in localStorage. Unrelated to
 * the per-manager back/forward `history` array in useFileManagerStore.
 *
 * A directory visit is credited at most once per stay, and only when the user
 * actually works there: after DWELL_MS in the directory, or immediately when
 * a file is opened in it — whichever comes first. Directories passed through
 * on the way (cstrike → addons → amxmodx → configs) are never recorded.
 *
 * useFileManagerStore calls into this store; this store must not import it
 * back. It also survives fm.resetState() untouched — only the pending dwell
 * timer is cancelled on unmount.
 */
export const useHistoryStore = defineStore('fm-history', () => {
    const settings = useSettingsStore()

    // State for the active storage key only; swapped by ensureKey().
    const state = ref(createEmptyState())
    // Bumped when the popover opens so time-dependent output (relative
    // labels, the recent window) recomputes without any entry changing.
    const nowTick = ref(Date.now())

    const storage = createStorage()
    let currentKey = null
    // The current directory stay: { disk, path, timerId, credited }.
    let session = null

    function resolveServerId() {
        if (settings.serverId) return String(settings.serverId)

        // Fallback for embedders that only pass baseUrl ('/api/file-manager/3').
        return String(settings.baseUrl || '')
            .replace(/\/+$/, '')
            .split('/')
            .pop()
    }

    // selectedDirectory uses null for the disk root, API dirnames use ''.
    function normalizePath(path) {
        return path == null ? '' : String(path)
    }

    function ensureKey(disk) {
        if (!disk) return false

        const key = storageKey(resolveServerId(), disk)
        if (key !== currentKey) {
            currentKey = key
            state.value = loadState(storage.get(key))
        }

        return true
    }

    function persist() {
        if (!currentKey) return
        storage.set(currentKey, JSON.stringify(state.value))
    }

    function touchNow() {
        nowTick.value = Date.now()
    }

    function cancelPending() {
        if (session && session.timerId) {
            clearTimeout(session.timerId)
        }
        session = null
    }

    function onDirectoryChanged({ disk, path }) {
        const dir = normalizePath(path)
        if (session && session.disk === disk && session.path === dir) {
            // Reload of the same directory (F5, current breadcrumb) keeps the
            // running stay — no restart, no second credit.
            return
        }

        cancelPending()
        if (!ensureKey(disk)) return

        const current = { disk, path: dir, timerId: null, credited: false }
        session = current

        // The root is one click away — never recorded, so no timer either.
        if (dir === '') return

        current.timerId = setTimeout(() => {
            current.timerId = null
            current.credited = true
            recordVisit(state.value, 'dir', dir, Date.now())
            persist()
        }, DWELL_MS)
    }

    /**
     * A file was opened (editor, preview, player, PDF). Credits the parent
     * directory instantly if its stay was not credited yet, and bumps the
     * file entry itself unless recordFile is false (picker/fileCallback mode).
     */
    function noteFileOpened({ disk, path, dirname, recordFile = true }) {
        if (!ensureKey(disk)) return

        const dir = normalizePath(dirname)
        const now = Date.now()

        if (session && session.disk === disk && session.path === dir && !session.credited && dir !== '') {
            if (session.timerId) {
                clearTimeout(session.timerId)
                session.timerId = null
            }
            session.credited = true
            recordVisit(state.value, 'dir', dir, now)
        }

        if (recordFile && path) {
            recordVisit(state.value, 'file', String(path), now)
        }

        persist()
    }

    function onItemsDeleted({ disk, items }) {
        if (!ensureKey(disk)) return

        for (const item of items) {
            removeDeleted(state.value, { path: String(item.path), type: item.type })
        }

        persist()
    }

    function onItemRenamed({ disk, type, oldPath, newPath }) {
        if (!ensureKey(disk)) return

        moveRenamed(state.value, { type, oldPath: String(oldPath), newPath: String(newPath) })
        persist()
    }

    // A history entry pointed at something that no longer exists.
    function dropStale(kind, path) {
        dropEntry(state.value, kind, path)
        persist()
    }

    function setView(view) {
        if (state.value.view === view) return
        state.value.view = view
        persist()
    }

    // Controls the history button: no entries at all — no button.
    const hasAnyEntries = computed(
        () => Object.keys(state.value.entries.dir).length > 0
            || Object.keys(state.value.entries.file).length > 0
    )

    const recentDirs = computed(() => topEntries(state.value, 'dir', 'recent', nowTick.value))
    const recentFiles = computed(() => topEntries(state.value, 'file', 'recent', nowTick.value))
    const hasRecent = computed(() => recentDirs.value.length > 0 || recentFiles.value.length > 0)

    // With nothing inside the recent window the tab switcher is pointless:
    // the popover falls through to the frequent view.
    const effectiveView = computed(() => (hasRecent.value ? state.value.view : 'frequent'))

    const dirsTop = computed(() => topEntries(state.value, 'dir', effectiveView.value, nowTick.value))
    const filesTop = computed(() => topEntries(state.value, 'file', effectiveView.value, nowTick.value))

    return {
        state,
        nowTick,
        hasAnyEntries,
        hasRecent,
        effectiveView,
        dirsTop,
        filesTop,
        ensureKey,
        touchNow,
        cancelPending,
        onDirectoryChanged,
        noteFileOpened,
        onItemsDeleted,
        onItemRenamed,
        dropStale,
        setView,
    }
})
