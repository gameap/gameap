import { defineStore } from 'pinia'
import { ref, computed, reactive } from 'vue'
import GET from '../http/get.js'
import POST from '../http/post.js'
import { uploadFileChunked } from '../http/upload-session.js'
import { downloadDirectoryArchive } from '../http/download-archive.js'
import { downloadSingleFile } from '../http/download-file.js'
import { StreamDownloadError } from '../http/download-stream.js'
import { detectConflicts, joinPath, dirOf } from '../http/upload-conflicts.js'
import { queryVariants } from '../keyboardLayout.js'
import { foldText } from '../textFold.js'
import { useSettingsStore } from './useSettingsStore.js'
import { useMessagesStore } from './useMessagesStore.js'
import { useModalStore } from './useModalStore.js'
import { useTranslate } from '../composables/useTranslate.js'
import { notification } from '@/parts/dialogs.js'

const FILE_CONCURRENCY = 3
const MKDIR_CONCURRENCY = 5

function addRenameSuffix(name, taken, splitExtension = true) {
    const dot = splitExtension ? name.lastIndexOf('.') : -1
    const base = dot > 0 ? name.slice(0, dot) : name
    const ext = dot > 0 ? name.slice(dot) : ''
    let i = 1
    let candidate = `${base}_${i}${ext}`
    while (taken.has(candidate)) {
        i += 1
        candidate = `${base}_${i}${ext}`
    }

    return candidate
}

function normalizeComparePath(p) {
    const normalized = String(p ?? '')
        .replace(/\\/g, '/')
        .replace(/^\/+/, '')
        .replace(/\/+$/, '')

    return normalized === '.' ? '' : normalized
}

function basenameOf(p) {
    const normalized = normalizeComparePath(p)

    return normalized.slice(normalized.lastIndexOf('/') + 1)
}

async function runPool(items, concurrency, worker) {
    let cursor = 0
    async function loop() {
        while (cursor < items.length) {
            const idx = cursor
            cursor += 1
            await worker(items[idx], idx)
        }
    }
    const c = Math.min(concurrency, items.length || 1)
    await Promise.all(Array.from({ length: c }, () => loop()))
}

function createManagerState() {
    return {
        selectedDisk: null,
        selectedDirectory: null,
        directories: [],
        files: [],
        loading: false,
        error: null,
        selected: {
            directories: [],
            files: [],
        },
        lastSelectedPath: null,
        sort: {
            field: 'name',
            direction: 'up',
        },
        history: [null],
        historyPointer: 0,
    }
}

function normalizeFetchError(err) {
    if (err && err.response) {
        return {
            status: err.response.status,
            message: err.response.data?.message || err.response.statusText || 'Request failed',
        }
    }
    if (err && err.request) {
        return { status: 0, message: err.request.statusText || 'Network error' }
    }

    return { status: 0, message: err?.message || 'Unknown error' }
}

export const useFileManagerStore = defineStore('fm', () => {
    // Root state
    const activeManager = ref('left')
    const clipboard = ref({
        type: null,
        disk: null,
        directories: [],
        files: [],
    })
    const disks = ref({})
    const fileCallback = ref(null)
    const fullScreen = ref(false)

    // Search state is root-level and applies to the active manager
    const searchOpen = ref(false)
    const searchQuery = ref('')
    const searchCursor = ref(0)
    const searchFocusRequestId = ref(0)
    const searchScrollTick = ref(0)

    // Manager states (left/right)
    const left = reactive(createManagerState())
    const right = reactive(createManagerState())

    // Helper to get manager by name
    function getManager(name) {
        return name === 'left' ? left : right
    }

    // Root getters
    const diskList = computed(() => Object.keys(disks.value))
    const inactiveManager = computed(() => (activeManager.value === 'left' ? 'right' : 'left'))
    const selectedDisk = computed(() => getManager(activeManager.value).selectedDisk)
    const selectedDirectory = computed(() => getManager(activeManager.value).selectedDirectory)

    // Manager getters
    function getFiles(managerName) {
        const settings = useSettingsStore()
        const manager = getManager(managerName)
        if (settings.hiddenFiles) {
            return manager.files
        }
        return manager.files.filter((f) => f.basename.match(/^([^.]).*/i))
    }

    function getDirectories(managerName) {
        const settings = useSettingsStore()
        const manager = getManager(managerName)
        if (settings.hiddenFiles) {
            return manager.directories
        }
        return manager.directories.filter((d) => d.basename.match(/^([^.]).*/i))
    }

    function getFilesCount(managerName) {
        return getFiles(managerName).length
    }

    function getDirectoriesCount(managerName) {
        return getDirectories(managerName).length
    }

    function getFilesSize(managerName) {
        const files = getFiles(managerName)
        if (files.length) {
            return files.reduce((previous, current) => previous + Number(current.size), 0)
        }
        return 0
    }

    function getSelectedList(managerName) {
        const manager = getManager(managerName)
        const selectedDirectories = manager.directories.filter((directory) =>
            manager.selected.directories.includes(directory.path)
        )
        const selectedFiles = manager.files.filter((file) =>
            manager.selected.files.includes(file.path)
        )
        return selectedDirectories.concat(selectedFiles)
    }

    function getSelectedCount(managerName) {
        return getSelectedList(managerName).length
    }

    function getSelectedFilesSize(managerName) {
        const manager = getManager(managerName)
        const selectedFiles = manager.files.filter((file) =>
            manager.selected.files.includes(file.path)
        )
        if (selectedFiles.length) {
            return selectedFiles.reduce((previous, current) => previous + Number(current.size), 0)
        }
        return 0
    }

    function getBreadcrumb(managerName) {
        const manager = getManager(managerName)
        if (manager.selectedDirectory) {
            return manager.selectedDirectory.split('/')
        }
        return null
    }

    function directoryExist(managerName, basename) {
        const manager = getManager(managerName)
        return manager.directories.some((el) => el.basename === basename)
    }

    function fileExist(managerName, basename) {
        const manager = getManager(managerName)
        return manager.files.some((el) => el.basename === basename)
    }

    // Computed for active manager
    const selectedItems = computed(() => getSelectedList(activeManager.value))

    // Manager mutations (as actions)
    function setManagerDisk(managerName, disk) {
        getManager(managerName).selectedDisk = disk
    }

    function setManagerDirectory(managerName, directory) {
        getManager(managerName).selectedDirectory = directory
    }

    function setManagerContent(managerName, { directories, files }) {
        const manager = getManager(managerName)
        manager.directories = directories
        manager.files = files
    }

    function setManagerLoading(managerName, loading) {
        getManager(managerName).loading = loading
    }

    function setManagerError(managerName, error) {
        getManager(managerName).error = error
    }

    function getManagerLoading(managerName) {
        return getManager(managerName).loading
    }

    function getManagerError(managerName) {
        return getManager(managerName).error
    }

    function addToSelection(managerName, { type, path }) {
        const manager = getManager(managerName)
        manager.selected[type].push(path)
        manager.lastSelectedPath = { type, path }
    }

    function removeFromSelection(managerName, { type, path }) {
        const manager = getManager(managerName)
        const itemIndex = manager.selected[type].indexOf(path)
        if (itemIndex !== -1) {
            manager.selected[type].splice(itemIndex, 1)
        }
        manager.lastSelectedPath = { type, path }
    }

    function changeSelected(managerName, { type, path }) {
        const manager = getManager(managerName)
        manager.selected.directories = []
        manager.selected.files = []
        manager.selected[type].push(path)
        manager.lastSelectedPath = { type, path }
    }

    function clearSelection(managerName) {
        const manager = getManager(managerName)
        manager.selected.directories = []
        manager.selected.files = []
        manager.lastSelectedPath = null
    }

    function setAnchor(managerName, { type, path }) {
        getManager(managerName).lastSelectedPath = { type, path }
    }

    function selectRange(managerName, { fromAnchor, toItem, visible }) {
        if (!fromAnchor || !toItem || !Array.isArray(visible) || visible.length === 0) return

        const idxA = visible.findIndex((i) => i.type === fromAnchor.type && i.path === fromAnchor.path)
        const idxB = visible.findIndex((i) => i.type === toItem.type && i.path === toItem.path)
        if (idxA < 0 || idxB < 0) return

        const [lo, hi] = idxA < idxB ? [idxA, idxB] : [idxB, idxA]
        const range = visible.slice(lo, hi + 1)

        const manager = getManager(managerName)
        manager.selected.directories = range.filter((i) => i.type === 'directories').map((i) => i.path)
        manager.selected.files = range.filter((i) => i.type === 'files').map((i) => i.path)
        manager.lastSelectedPath = { type: toItem.type, path: toItem.path }
    }

    function selectAllVisible(managerName) {
        const manager = getManager(managerName)
        manager.selected.directories = getDirectories(managerName).map((d) => d.path)
        manager.selected.files = getFiles(managerName).map((f) => f.path)
    }

    function addNewFile(managerName, newFile) {
        getManager(managerName).files.push(newFile)
    }

    function setFile(managerName, file) {
        const manager = getManager(managerName)
        const itemIndex = manager.files.findIndex((el) => el.basename === file.basename)
        if (itemIndex !== -1) {
            manager.files[itemIndex] = file
        }
    }

    function addNewDirectory(managerName, newDirectory) {
        getManager(managerName).directories.push(newDirectory)
    }

    function setSort(managerName, { field, direction }) {
        const manager = getManager(managerName)
        manager.sort.field = field
        manager.sort.direction = direction
    }

    function resetSortSettings(managerName) {
        const manager = getManager(managerName)
        manager.sort.field = 'name'
        manager.sort.direction = 'up'
    }

    function addToHistory(managerName, path) {
        const manager = getManager(managerName)
        if (manager.historyPointer < manager.history.length - 1) {
            manager.history.splice(manager.historyPointer + 1)
        }
        manager.history.push(path)
        manager.historyPointer += 1
    }

    function pointerBack(managerName) {
        getManager(managerName).historyPointer -= 1
    }

    function pointerForward(managerName) {
        getManager(managerName).historyPointer += 1
    }

    function resetHistory(managerName) {
        const manager = getManager(managerName)
        manager.history = [null]
        manager.historyPointer = 0
    }

    // Sorting mutations
    function sortByName(managerName) {
        const manager = getManager(managerName)
        if (manager.sort.direction === 'up') {
            manager.directories.sort((a, b) => a.basename.localeCompare(b.basename))
            manager.files.sort((a, b) => a.basename.localeCompare(b.basename))
        } else {
            manager.directories.sort((a, b) => b.basename.localeCompare(a.basename))
            manager.files.sort((a, b) => b.basename.localeCompare(a.basename))
        }
    }

    function sortBySize(managerName) {
        const manager = getManager(managerName)
        manager.directories.sort((a, b) => a.basename.localeCompare(b.basename))
        if (manager.sort.direction === 'up') {
            manager.files.sort((a, b) => a.size - b.size)
        } else {
            manager.files.sort((a, b) => b.size - a.size)
        }
    }

    function sortByType(managerName) {
        const manager = getManager(managerName)
        manager.directories.sort((a, b) => a.basename.localeCompare(b.basename))
        if (manager.sort.direction === 'up') {
            manager.files.sort((a, b) => a.extension.localeCompare(b.extension))
        } else {
            manager.files.sort((a, b) => b.extension.localeCompare(a.extension))
        }
    }

    function sortByDate(managerName) {
        const manager = getManager(managerName)
        if (manager.sort.direction === 'up') {
            manager.directories.sort((a, b) => a.timestamp - b.timestamp)
            manager.files.sort((a, b) => a.timestamp - b.timestamp)
        } else {
            manager.directories.sort((a, b) => b.timestamp - a.timestamp)
            manager.files.sort((a, b) => b.timestamp - a.timestamp)
        }
    }

    // Root mutations
    function setDisks(newDisks) {
        disks.value = newDisks
    }

    function setClipboard({ type, disk, directories, files }) {
        clipboard.value.type = type
        clipboard.value.disk = disk
        clipboard.value.directories = directories
        clipboard.value.files = files
    }

    function truncateClipboard({ type, path }) {
        const itemIndex = clipboard.value[type].indexOf(path)
        if (itemIndex !== -1) {
            clipboard.value[type].splice(itemIndex, 1)
        }
        if (!clipboard.value.directories.length && !clipboard.value.files.length) {
            clipboard.value.type = null
        }
    }

    function resetClipboard() {
        clipboard.value.type = null
        clipboard.value.disk = null
        clipboard.value.directories = []
        clipboard.value.files = []
    }

    function setActiveManager(managerName) {
        activeManager.value = managerName
    }

    function setFileCallBack(callback) {
        fileCallback.value = callback
    }

    function screenToggle() {
        fullScreen.value = !fullScreen.value
    }

    // Search: names follow the visible order (directories first, then files),
    // the same index space as TableView row refs and useManager flatVisible.
    const foldedNames = computed(() => [
        ...getDirectories(activeManager.value).map((directory) => foldText(directory.basename)),
        ...getFiles(activeManager.value).map((file) => foldText(file.basename)),
    ])

    function collectMatches(query) {
        const needle = foldText(query)
        const matches = []

        foldedNames.value.forEach((name, index) => {
            if (name.includes(needle)) {
                matches.push(index)
            }
        })

        return matches
    }

    // Other keyboard layouts are only a fallback: as long as the typed text
    // finds something, a wrong-layout reading never steals the results.
    const searchResult = computed(() => {
        if (!searchOpen.value || searchQuery.value === '') {
            return { query: '', matches: [] }
        }

        const variants = queryVariants(searchQuery.value)
        for (const variant of variants) {
            const matches = collectMatches(variant)
            if (matches.length) {
                return { query: variant, matches }
            }
        }

        return { query: variants[0], matches: [] }
    })

    const searchMatches = computed(() => searchResult.value.matches)
    const effectiveSearchQuery = computed(() => searchResult.value.query)

    const currentSearchMatch = computed(() => {
        const matches = searchMatches.value
        if (matches.length === 0) return -1

        return matches[Math.min(searchCursor.value, matches.length - 1)]
    })

    function openSearch() {
        searchOpen.value = true
        searchFocusRequestId.value += 1
    }

    function openSearchWithQuery(query) {
        searchOpen.value = true
        setSearchQuery(query)
        searchFocusRequestId.value += 1
    }

    function closeSearch() {
        searchOpen.value = false
        searchQuery.value = ''
        searchCursor.value = 0
    }

    function toggleSearch() {
        if (searchOpen.value) {
            closeSearch()
        } else {
            openSearch()
        }
    }

    function setSearchQuery(query) {
        searchQuery.value = query
        searchCursor.value = 0
        searchScrollTick.value += 1
    }

    function appendSearchChar(char) {
        setSearchQuery(searchQuery.value + char)
        searchFocusRequestId.value += 1
    }

    function searchNext() {
        const len = searchMatches.value.length
        if (len === 0) return

        searchCursor.value = (Math.min(searchCursor.value, len - 1) + 1) % len
        searchScrollTick.value += 1
    }

    function searchPrev() {
        const len = searchMatches.value.length
        if (len === 0) return

        searchCursor.value = (Math.min(searchCursor.value, len - 1) - 1 + len) % len
        searchScrollTick.value += 1
    }

    function resetSearchCursor() {
        searchCursor.value = 0
        searchScrollTick.value += 1
    }

    // Manager actions
    async function selectDirectory(managerName, { path, history }) {
        const manager = getManager(managerName)

        setManagerContent(managerName, { directories: [], files: [] })
        setManagerError(managerName, null)
        setManagerLoading(managerName, true)
        try {
            const response = await GET.content(manager.selectedDisk, path)
            if (response.data.result.status === 'success') {
                clearSelection(managerName)
                resetSortSettings(managerName)
                setManagerContent(managerName, response.data)
                setManagerDirectory(managerName, path)

                if (managerName === activeManager.value) {
                    resetSearchCursor()
                }

                if (history) {
                    addToHistory(managerName, path)
                }
            } else {
                setManagerError(managerName, {
                    status: 0,
                    message: response.data.result.message || 'Failed to load directory',
                })
            }
        } catch (err) {
            setManagerError(managerName, normalizeFetchError(err))
        } finally {
            setManagerLoading(managerName, false)
        }
    }

    async function refreshDirectory(managerName, retried = false) {
        const manager = getManager(managerName)

        setManagerError(managerName, null)
        setManagerLoading(managerName, true)
        try {
            const response = await GET.content(manager.selectedDisk, manager.selectedDirectory)
            clearSelection(managerName)
            resetSortSettings(managerName)
            resetHistory(managerName)

            if (manager.selectedDirectory) {
                addToHistory(managerName, manager.selectedDirectory)
            }

            if (response.data.result.status === 'success') {
                setManagerContent(managerName, response.data)

                if (managerName === activeManager.value) {
                    resetSearchCursor()
                }
            } else if (response.data.result.status === 'danger' && !retried) {
                setManagerDirectory(managerName, null)
                await refreshDirectory(managerName, true)
            } else {
                setManagerError(managerName, {
                    status: 0,
                    message: response.data.result.message || 'Failed to load directory',
                })
            }
        } catch (err) {
            setManagerError(managerName, normalizeFetchError(err))
        } finally {
            setManagerLoading(managerName, false)
        }
    }

    async function historyBack(managerName) {
        const manager = getManager(managerName)
        const path = manager.history[manager.historyPointer - 1]
        await selectDirectory(managerName, { path, history: false })
        pointerBack(managerName)
    }

    async function historyForward(managerName) {
        const manager = getManager(managerName)
        const path = manager.history[manager.historyPointer + 1]
        await selectDirectory(managerName, { path, history: false })
        pointerForward(managerName)
    }

    function sortBy(managerName, { field, direction }) {
        const manager = getManager(managerName)

        if (manager.sort.field === field && !direction) {
            setSort(managerName, {
                field,
                direction: manager.sort.direction === 'up' ? 'down' : 'up',
            })
        } else if (direction) {
            setSort(managerName, { field, direction })
        } else {
            setSort(managerName, { field, direction: 'up' })
        }

        switch (field) {
            case 'name':
                sortByName(managerName)
                break
            case 'size':
                sortBySize(managerName)
                break
            case 'type':
                sortByType(managerName)
                break
            case 'date':
                sortByDate(managerName)
                break
            default:
                break
        }
    }

    // Root actions
    async function initializeApp() {
        const settings = useSettingsStore()

        const response = await GET.initialize()
        if (response.data.result.status === 'success') {
            settings.initSettings(response.data.config)
            setDisks(response.data.config.disks)

            let leftDisk = response.data.config.leftDisk
                ? response.data.config.leftDisk
                : diskList.value[0]
            let leftPath = response.data.config.leftPath

            if (window.location.search) {
                const params = new URLSearchParams(window.location.search)
                if (params.get('leftDisk')) leftDisk = params.get('leftDisk')
                if (params.get('leftPath')) leftPath = params.get('leftPath')
            }

            setManagerDisk('left', leftDisk)

            if (leftPath) {
                setManagerDirectory('left', leftPath)
                addToHistory('left', leftPath)
            }

            getLoadContent({ manager: 'left', disk: leftDisk, path: leftPath })
        }
    }

    async function getLoadContent({ manager, disk, path }) {
        setManagerError(manager, null)
        setManagerLoading(manager, true)
        try {
            const response = await GET.content(disk, path)
            if (response.data.result.status === 'success') {
                setManagerContent(manager, response.data)
            } else {
                setManagerError(manager, {
                    status: 0,
                    message: response.data.result.message || 'Failed to load directory',
                })
            }
        } catch (err) {
            setManagerError(manager, normalizeFetchError(err))
        } finally {
            setManagerLoading(manager, false)
        }
    }

    async function selectDiskAction({ disk, manager: managerName }) {
        const response = await GET.selectDisk(disk)
        if (response.data.result.status === 'success') {
            setManagerDisk(managerName, disk)
            resetHistory(managerName)
            selectDirectory(managerName, { path: null, history: false })
        }
    }

    async function createFile(fileName) {
        const currentDirectory = selectedDirectory.value

        const response = await POST.createFile(selectedDisk.value, currentDirectory, fileName)
        updateContent({
            response,
            oldDir: currentDirectory,
            commitName: 'addNewFile',
            type: 'file',
        })
        return response
    }

    function getFile({ disk, path }) {
        return GET.getFile(disk, path)
    }

    function getFileArrayBuffer({ disk, path }) {
        return GET.getFileArrayBuffer(disk, path)
    }

    async function updateFileAction(formData) {
        const response = await POST.updateFile(formData)
        updateContent({
            response,
            oldDir: selectedDirectory.value,
            commitName: 'updateFile',
            type: 'file',
        })
        return response
    }

    async function createDirectory(name) {
        const currentDirectory = selectedDirectory.value

        const response = await POST.createDirectory({
            disk: selectedDisk.value,
            path: currentDirectory,
            name,
        })
        updateContent({
            response,
            oldDir: currentDirectory,
            commitName: 'addNewDirectory',
            type: 'directory',
        })
        return response
    }

    function uniqueDirsFromEntries(entries, emptyDirs) {
        const dirs = new Set()
        for (const e of entries) {
            let d = dirOf(e.relPath)
            while (d) {
                dirs.add(d)
                const i = d.lastIndexOf('/')
                d = i === -1 ? '' : d.slice(0, i)
            }
        }
        for (const d of emptyDirs) {
            let cur = d
            while (cur) {
                dirs.add(cur)
                const i = cur.lastIndexOf('/')
                cur = i === -1 ? '' : cur.slice(0, i)
            }
        }
        const sorted = Array.from(dirs)
        sorted.sort((a, b) => a.split('/').length - b.split('/').length)

        return sorted
    }

    async function preparePreflight({ entries, emptyDirs }) {
        const messages = useMessagesStore()
        const rootPath = selectedDirectory.value || ''
        const abortController = new AbortController()
        messages.initUploadProgress({
            rootPath,
            files: entries,
            emptyDirs,
            abortController,
        })

        const result = await detectConflicts({
            disk: selectedDisk.value,
            rootPath,
            files: entries,
            emptyDirs,
        })
        messages.setUploadListings(result.listings)
        messages.applyConflicts(result)
        messages.setUploadStatus('review')
    }

    async function upload({ entries, emptyDirs = [] }) {
        const list = Array.from(entries || [])
        if (list.length === 0 && emptyDirs.length === 0) return { data: { result: { status: 'warning' } } }
        await preparePreflight({ entries: list, emptyDirs })

        return { data: { result: { status: 'review' } } }
    }

    async function ensureDirectories(rootPath, dirRels) {
        if (dirRels.length === 0) return
        await runPool(dirRels, MKDIR_CONCURRENCY, async (relDir) => {
            const target = joinPath(rootPath, relDir)
            const slash = target.lastIndexOf('/')
            const parent = slash === -1 ? '' : target.slice(0, slash)
            const name = slash === -1 ? target : target.slice(slash + 1)
            try {
                await POST.createDirectory(
                    {
                        disk: selectedDisk.value,
                        path: parent,
                        name,
                    },
                    { silent: true },
                )
            } catch (err) {
                /* merge: ignore creation errors (already exists) */
            }
        })
    }

    async function runFilePool(rootPath, files, abortController) {
        const messages = useMessagesStore()
        await runPool(files, FILE_CONCURRENCY, async (file) => {
            if (abortController.signal.aborted) return
            const targetDir = joinPath(rootPath, file.dirPath)
            const filename = file.renamedTo || file.name
            try {
                await uploadFileChunked(file._raw, {
                    path: targetDir,
                    filename,
                    signal: abortController.signal,
                    onPhase: (phase) => messages.setFilePhase({ index: file.index, phase }),
                    onProgress: ({ phase, loaded }) => messages.setFileProgress({ index: file.index, loaded, phase }),
                })
                messages.setFilePhase({ index: file.index, phase: 'done' })
            } catch (err) {
                const code = err && err.code ? err.code : 'unknown'
                if (code !== 'aborted') {
                    console.error('[filemanager] upload failed:', filename, code, err)
                }
                messages.setFileError({
                    index: file.index,
                    error: code,
                    detail: err && err.message ? err.message : null,
                })
            }
        })
    }

    async function startUpload() {
        const messages = useMessagesStore()
        const up = messages.uploadProgress
        const rootPath = up.rootPath || ''
        const abortController = up.abortController || new AbortController()
        if (!up.abortController) up.abortController = abortController

        const renamesUsed = new Map()
        const fileItems = []
        for (let i = 0; i < up.files.length; i += 1) {
            const meta = up.files[i]
            const raw = messages.getRawFile(i)
            const action = meta.action
            if ((meta.conflict === 'file' || meta.conflict === 'dir-vs-file') && action === 'skip') {
                messages.markFilesSkipped((f) => f.index === meta.index)
                continue
            }
            if (meta.conflict === 'dir-vs-file') {
                messages.setFileError({ index: meta.index, error: 'dir_vs_file' })
                continue
            }
            let renamedTo = null
            if (action === 'rename' && meta.conflict === 'file') {
                const dirKey = meta.dirPath
                if (!renamesUsed.has(dirKey)) {
                    const taken = new Set()
                    const absDir = joinPath(rootPath, dirKey)
                    const serverListing = messages.getDirListing(absDir)
                    if (serverListing) {
                        for (const existing of serverListing.keys()) taken.add(existing)
                    }
                    renamesUsed.set(dirKey, taken)
                }
                renamesUsed.get(dirKey).add(meta.name)
                renamedTo = addRenameSuffix(meta.name, renamesUsed.get(dirKey))
                renamesUsed.get(dirKey).add(renamedTo)
                messages.setFileAction({ index: meta.index, action: 'rename', renamedTo })
            }
            fileItems.push({ ...meta, renamedTo, _raw: raw })
        }

        messages.setUploadStatus('mkdir')
        const dirRels = uniqueDirsFromEntries(
            up.files.map((f) => ({ relPath: f.relPath })),
            up.emptyDirs || [],
        )
        await ensureDirectories(rootPath, dirRels)
        if (abortController.signal.aborted) {
            messages.setUploadStatus('cancelled')

            return { data: { result: { status: 'cancelled' } } }
        }

        messages.setUploadStatus('uploading')
        await runFilePool(rootPath, fileItems, abortController)

        if (abortController.signal.aborted) {
            messages.setUploadStatus('cancelled')

            return { data: { result: { status: 'cancelled' } } }
        }

        const failed = up.totals.failedFiles
        messages.setUploadStatus(failed > 0 ? 'partial' : 'completed')

        if (rootPath === (selectedDirectory.value || '')) {
            await refreshManagers()
        }

        return { data: { result: { status: failed > 0 ? 'warning' : 'success' } } }
    }

    function cancelUpload() {
        const messages = useMessagesStore()
        const up = messages.uploadProgress
        if (up.abortController) up.abortController.abort()
        messages.setUploadStatus('cancelled')
    }

    async function retryFailed() {
        const messages = useMessagesStore()
        const up = messages.uploadProgress
        const newController = new AbortController()
        up.abortController = newController
        messages.resetFailedToPending()
        messages.setUploadStatus('uploading')

        const items = []
        for (const meta of up.files) {
            if (meta.phase !== 'pending') continue
            const raw = messages.getRawFile(meta.index)
            items.push({ ...meta, _raw: raw })
        }
        await runFilePool(up.rootPath || '', items, newController)
        const failed = up.totals.failedFiles
        messages.setUploadStatus(failed > 0 ? 'partial' : 'completed')
        if ((up.rootPath || '') === (selectedDirectory.value || '')) {
            await refreshManagers()
        }
    }

    function clearUpload() {
        const messages = useMessagesStore()
        messages.clearUploadProgress()
    }

    // Shared progress/cancel bookkeeping for single-file and archive downloads. Every store update
    // is gated on isCurrent(): a cancelled or finished download must never touch the state of a
    // download the user started afterwards, and a user cancel just clears the bar (no error flash).
    async function trackDownload({ kind, filename, label, run }) {
        const messages = useMessagesStore()
        const abortController = new AbortController()
        const { signal } = abortController
        messages.startArchiveDownload({ filename, abortController, kind })

        const isCurrent = () => messages.archiveDownload.abortController === abortController
        const clearLater = (ms) => {
            setTimeout(() => {
                if (isCurrent()) messages.clearArchiveDownload()
            }, ms)
        }

        try {
            await run({
                signal,
                onPhase: (phase) => {
                    if (isCurrent()) messages.setArchivePhase(phase)
                },
                onProgress: (progress) => {
                    if (isCurrent()) messages.setArchiveProgress(progress)
                },
            })
            clearLater(5000)
        } catch (err) {
            const code = err instanceof StreamDownloadError ? err.code : 'unknown'
            if (code === 'aborted' || signal.aborted) {
                if (isCurrent()) messages.clearArchiveDownload()
                throw err
            }
            console.error(`[${label}] failed`, err)
            const message = err && err.message ? err.message : 'unknown'
            if (isCurrent()) messages.setArchiveError({ code, message })
            clearLater(8000)
            throw err
        }
    }

    async function download({ disk, path, filename }) {
        const settings = useSettingsStore()
        const fileName = filename || (path || '').split('/').filter(Boolean).pop() || 'file'

        await trackDownload({
            kind: 'file',
            filename: fileName,
            label: 'download',
            run: ({ signal, onPhase, onProgress }) => downloadSingleFile({
                baseUrl: settings.baseUrl,
                disk,
                path,
                filename: fileName,
                headers: settings.headers,
                onPhase,
                onProgress,
                signal,
            }),
        }).catch(() => {
            /* errors are surfaced via the messages store */
        })
    }

    async function downloadDirectory({ disk, path, filename, compress = 0 }) {
        const settings = useSettingsStore()
        const archiveName = filename || `${(path || '').split('/').filter(Boolean).pop() || 'archive'}.zip`

        await trackDownload({
            kind: 'archive',
            filename: archiveName,
            label: 'downloadDirectory',
            run: ({ signal, onPhase, onProgress }) => downloadDirectoryArchive({
                baseUrl: settings.baseUrl,
                disk,
                path,
                filename: archiveName,
                compress,
                headers: settings.headers,
                onPhase,
                onProgress,
                signal,
            }),
        })
    }

    function sanitizeArchiveName(raw) {
        const cleaned = String(raw || '').trim().replace(/\s+/g, '_').replace(/[\\/:*?"<>|]/g, '_')

        return cleaned || 'archive'
    }

    function isRootPath(p) {
        return !p || p === '/' || p === '.' || p === ''
    }

    async function downloadCurrentDirectory() {
        const settings = useSettingsStore()
        const manager = getManager(activeManager.value)
        const currentDisk = manager.selectedDisk
        if (!currentDisk) return

        const currentPath = manager.selectedDirectory
        const rootCandidate = isRootPath(currentPath)

        let archiveName
        if (rootCandidate && settings.serverName) {
            archiveName = `${sanitizeArchiveName(settings.serverName)}.zip`
        } else {
            const segments = (currentPath || '').split('/').filter(Boolean)
            const last = segments.length > 0 ? segments[segments.length - 1] : ''
            archiveName = `${sanitizeArchiveName(last || settings.serverName || 'archive')}.zip`
        }

        await downloadDirectory({
            disk: currentDisk,
            path: rootCandidate ? '/' : currentPath,
            filename: archiveName,
        }).catch(() => {
            /* errors are surfaced via the messages store */
        })
    }

    function cancelDirectoryDownload() {
        const messages = useMessagesStore()
        const ctl = messages.archiveDownload.abortController
        if (ctl) {
            try {
                ctl.abort()
            } catch (e) {
                console.warn('[cancelDirectoryDownload] abort failed', e)
            }
        }
        messages.clearArchiveDownload()
    }

    async function deleteItems(items) {
        const response = await POST.delete({
            disk: selectedDisk.value,
            items,
        })

        if (response.data.result.status === 'success') {
            refreshManagers()
        }
        return response
    }

    async function paste() {
        const manager = getManager(activeManager.value)
        const { lang } = useTranslate()
        const destDir = normalizeComparePath(manager.selectedDirectory)
        const sameDisk = clipboard.value.disk === manager.selectedDisk

        const pastingIntoItself = sameDisk && clipboard.value.directories.some((dir) => {
            const src = normalizeComparePath(dir)

            return destDir === src || destDir.startsWith(`${src}/`)
        })
        if (pastingIntoItself) {
            notification({ content: lang.value.notifications.pasteIntoItself, type: 'error' })

            return
        }

        const inDestDir = (itemPath) => sameDisk && dirOf(normalizeComparePath(itemPath)) === destDir

        if (clipboard.value.type === 'cut') {
            const items = [...clipboard.value.directories, ...clipboard.value.files]
            if (items.length > 0 && items.every(inDestDir)) {
                notification({ content: lang.value.notifications.pasteSameDirectory, type: 'info' })

                return
            }
        }

        let names = null
        if (clipboard.value.type === 'copy') {
            // Hidden entries occupy names too, so collect from the raw
            // listings rather than the hiddenFiles-filtered getters.
            const taken = new Set([
                ...manager.directories.map((d) => d.basename),
                ...manager.files.map((f) => f.basename),
            ])
            // Null prototype: an item literally named "__proto__" must
            // still become an own enumerable key of the payload map.
            names = Object.create(null)
            for (const dir of clipboard.value.directories) {
                if (!inDestDir(dir)) {
                    continue
                }
                const renamed = addRenameSuffix(basenameOf(dir), taken, false)
                names[dir] = renamed
                taken.add(renamed)
            }
            for (const file of clipboard.value.files) {
                if (!inDestDir(file)) {
                    continue
                }
                const renamed = addRenameSuffix(basenameOf(file), taken)
                names[file] = renamed
                taken.add(renamed)
            }
            if (Object.keys(names).length === 0) {
                names = null
            }
        }

        const payload = {
            disk: manager.selectedDisk,
            path: manager.selectedDirectory,
            clipboard: clipboard.value,
        }
        if (names) {
            payload.names = names
        }

        const response = await POST.paste(payload)

        if (response.data.result.status === 'success') {
            refreshManagers()

            if (clipboard.value.type === 'cut') {
                resetClipboard()
            }
        }
    }

    async function rename({ type, newName, oldName }) {
        const response = await POST.rename({
            disk: selectedDisk.value,
            newName,
            oldName,
            type,
        })

        if (type === 'dir') {
            refreshAll()
        } else {
            refreshManagers()
        }
        return response
    }

    async function chmod({ items, mode }) {
        const response = await POST.chmod({
            disk: selectedDisk.value,
            items,
            mode,
        })

        if (response.data.result.status === 'success') {
            refreshManagers()
        }
        return response
    }

    function url({ disk, path }) {
        return GET.url(disk, path)
    }

    function toClipboard(type) {
        const manager = getManager(activeManager.value)
        if (getSelectedCount(activeManager.value)) {
            setClipboard({
                type,
                disk: manager.selectedDisk,
                directories: manager.selected.directories.slice(0),
                files: manager.selected.files.slice(0),
            })
        }
    }

    async function refreshManagers() {
        const promises = [refreshDirectory('left')]
        if (right.selectedDisk) {
            promises.push(refreshDirectory('right'))
        }
        return Promise.all(promises)
    }

    async function refreshAll() {
        return refreshManagers()
    }

    function repeatSort(managerName) {
        const manager = getManager(managerName)
        sortBy(managerName, {
            field: manager.sort.field,
            direction: manager.sort.direction,
        })
    }

    function updateContent({ response, oldDir, commitName, type }) {
        if (response.data.result.status === 'success' && oldDir === selectedDirectory.value) {
            if (commitName === 'addNewFile') {
                addNewFile(activeManager.value, response.data[type])
            } else if (commitName === 'updateFile') {
                setFile(activeManager.value, response.data[type])
            } else if (commitName === 'addNewDirectory') {
                addNewDirectory(activeManager.value, response.data[type])
            }

            repeatSort(activeManager.value)
        }
    }

    function resetState() {
        const modal = useModalStore()
        const messages = useMessagesStore()

        // left manager
        setManagerDisk('left', null)
        setManagerDirectory('left', null)
        setManagerContent('left', { directories: [], files: [] })
        clearSelection('left')
        resetSortSettings('left')
        resetHistory('left')

        // right manager
        setManagerDisk('right', null)
        setManagerDirectory('right', null)
        setManagerContent('right', { directories: [], files: [] })
        clearSelection('right')
        resetSortSettings('right')
        resetHistory('right')

        // modals
        modal.clearModal()

        // messages
        messages.clearActionResult()
        messages.clearProgress()
        messages.clearLoading()
        messages.clearErrors()

        // root state
        activeManager.value = 'left'
        clipboard.value = {
            type: null,
            disk: null,
            directories: [],
            files: [],
        }
        disks.value = {}
        fileCallback.value = null
        fullScreen.value = false
        closeSearch()
    }

    function openPDF({ disk, path }) {
        GET.getFileArrayBuffer(disk, path).then((response) => {
            const blob = new Blob([response.data], { type: 'application/pdf' })
            window.open(URL.createObjectURL(blob))
        })
    }

    return {
        // Root state
        activeManager,
        clipboard,
        disks,
        fileCallback,
        fullScreen,
        // Manager states
        left,
        right,
        getManager,
        // Root getters
        diskList,
        inactiveManager,
        selectedDisk,
        selectedDirectory,
        selectedItems,
        // Manager getters
        getFiles,
        getDirectories,
        getFilesCount,
        getDirectoriesCount,
        getFilesSize,
        getSelectedList,
        getSelectedCount,
        getSelectedFilesSize,
        getBreadcrumb,
        getManagerLoading,
        getManagerError,
        directoryExist,
        fileExist,
        // Root mutations
        setDisks,
        setClipboard,
        truncateClipboard,
        resetClipboard,
        setActiveManager,
        setFileCallBack,
        screenToggle,
        // Search
        searchOpen,
        searchQuery,
        searchCursor,
        searchFocusRequestId,
        searchScrollTick,
        searchMatches,
        effectiveSearchQuery,
        currentSearchMatch,
        openSearch,
        openSearchWithQuery,
        closeSearch,
        toggleSearch,
        setSearchQuery,
        appendSearchChar,
        searchNext,
        searchPrev,
        resetSearchCursor,
        // Manager mutations
        setManagerDisk,
        setManagerDirectory,
        setManagerContent,
        setManagerLoading,
        setManagerError,
        addToSelection,
        removeFromSelection,
        changeSelected,
        clearSelection,
        setAnchor,
        selectRange,
        selectAllVisible,
        addNewFile,
        setFile,
        addNewDirectory,
        setSort,
        resetSortSettings,
        addToHistory,
        pointerBack,
        pointerForward,
        resetHistory,
        sortByName,
        sortBySize,
        sortByType,
        sortByDate,
        // Manager actions
        selectDirectory,
        refreshDirectory,
        historyBack,
        historyForward,
        sortBy,
        // Root actions
        initializeApp,
        getLoadContent,
        selectDisk: selectDiskAction,
        createFile,
        getFile,
        getFileArrayBuffer,
        updateFile: updateFileAction,
        createDirectory,
        upload,
        startUpload,
        cancelUpload,
        retryFailed,
        clearUpload,
        download,
        downloadDirectory,
        downloadCurrentDirectory,
        cancelDirectoryDownload,
        delete: deleteItems,
        paste,
        rename,
        chmod,
        url,
        toClipboard,
        refreshManagers,
        refreshAll,
        repeatSort,
        updateContent,
        resetState,
        openPDF,
    }
})
