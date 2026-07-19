import { defineStore } from 'pinia'
import { ref, computed, markRaw } from 'vue'

function createInitialArchiveDownload() {
    return {
        status: 'idle',
        kind: 'archive',
        phase: null,
        filename: '',
        loaded: 0,
        total: 0,
        totalFiles: 0,
        skippedCount: 0,
        abortController: null,
        error: null,
    }
}

function createInitialUploadProgress() {
    return {
        status: 'idle',
        rootPath: null,
        abortController: null,
        defaultAction: null,
        hasConflicts: false,
        totals: {
            files: 0,
            bytes: 0,
            completedFiles: 0,
            failedFiles: 0,
            skippedFiles: 0,
            loadedBytes: 0,
        },
        dirs: {},
        files: [],
        emptyDirs: [],
    }
}

function dirOfRel(relPath) {
    const i = relPath.lastIndexOf('/')

    return i === -1 ? '' : relPath.slice(0, i)
}

function ensureDir(dirs, relPath) {
    if (!dirs[relPath]) {
        dirs[relPath] = {
            relPath,
            name: relPath === '' ? '' : relPath.slice(relPath.lastIndexOf('/') + 1),
            files: 0,
            completed: 0,
            failed: 0,
            skipped: 0,
            loaded: 0,
            size: 0,
            expanded: relPath === '',
        }
    }

    return dirs[relPath]
}

function dirChain(relPath) {
    const chain = ['']
    if (!relPath) return chain
    const parts = relPath.split('/')
    let cur = ''
    for (const p of parts) {
        cur = cur ? `${cur}/${p}` : p
        chain.push(cur)
    }

    return chain
}

export const useMessagesStore = defineStore('fm-messages', () => {
    const actionResult = ref({
        status: null,
        message: null,
    })
    const actionProgress = ref(0)
    const progressLabel = ref('')
    const loadingCount = ref(0)
    const errors = ref([])
    const uploadProgress = ref(createInitialUploadProgress())
    const uploadRawFiles = { current: [] }
    const uploadListings = { current: new Map() }
    const archiveDownload = ref(createInitialArchiveDownload())

    const loading = computed(() => loadingCount.value > 0)

    function setActionResult({ status, message }) {
        actionResult.value.status = status
        actionResult.value.message = message
    }

    function clearActionResult() {
        actionResult.value.status = null
        actionResult.value.message = null
    }

    function setProgress(progress, label) {
        actionProgress.value = progress
        if (label !== undefined) {
            progressLabel.value = label
        }
    }

    function clearProgress() {
        actionProgress.value = 0
        progressLabel.value = ''
    }

    function initUploadProgress({ rootPath = '', files = [], emptyDirs = [], abortController = null } = {}) {
        const next = createInitialUploadProgress()
        next.status = 'preflight'
        next.rootPath = rootPath
        next.abortController = abortController
        next.emptyDirs = emptyDirs.slice()

        const raws = []
        for (let i = 0; i < files.length; i += 1) {
            const f = files[i]
            const dir = dirOfRel(f.relPath)
            next.files.push({
                index: i,
                relPath: f.relPath,
                name: f.name,
                size: f.size,
                dirPath: dir,
                conflict: 'none',
                action: 'overwrite',
                renamedTo: null,
                phase: 'pending',
                loaded: 0,
                uploadedBytes: 0,
                error: null,
                errorDetail: null,
            })
            raws.push(f.file)
            for (const d of dirChain(dir)) {
                const node = ensureDir(next.dirs, d)
                node.files += 1
                node.size += f.size
            }
            next.totals.files += 1
            next.totals.bytes += f.size
        }
        for (const d of emptyDirs) {
            for (const part of dirChain(d)) {
                ensureDir(next.dirs, part)
            }
        }
        uploadRawFiles.current = raws
        uploadProgress.value = next
    }

    function getRawFile(index) {
        return uploadRawFiles.current[index] || null
    }

    function setUploadListings(listings) {
        uploadListings.current = listings || new Map()
    }

    function getDirListing(absDir) {
        return uploadListings.current.get(absDir) || null
    }

    function applyConflicts({ fileConflicts, dirConflicts }) {
        const up = uploadProgress.value
        let hasConflicts = false
        for (const file of up.files) {
            const c = fileConflicts.get(file.relPath) || 'none'
            file.conflict = c
            if (c === 'file' || c === 'dir-vs-file') hasConflicts = true
        }
        if (dirConflicts) {
            for (const [d, kind] of dirConflicts.entries()) {
                if (kind === 'file-vs-dir') hasConflicts = true
                const node = ensureDir(up.dirs, d)
                node.conflict = kind
            }
        }
        up.hasConflicts = hasConflicts
    }

    function setUploadStatus(status) {
        uploadProgress.value.status = status
    }

    function setDefaultAction(action) {
        const up = uploadProgress.value
        up.defaultAction = action
        for (const file of up.files) {
            if (file.conflict === 'file' || file.conflict === 'none') {
                file.action = action
            }
        }
    }

    function setFileAction({ index, action, renamedTo = null }) {
        const file = uploadProgress.value.files[index]
        if (!file) return
        file.action = action
        file.renamedTo = renamedTo
    }

    function fileUploadedContribution(file) {
        if (file.phase === 'done' || file.phase === 'completing') return file.size

        return Math.min(file.uploadedBytes || 0, file.size)
    }

    function recomputeDirAggregates() {
        const up = uploadProgress.value
        for (const key of Object.keys(up.dirs)) {
            const d = up.dirs[key]
            d.completed = 0
            d.failed = 0
            d.skipped = 0
            d.loaded = 0
        }
        let totalLoaded = 0
        let totalCompleted = 0
        let totalFailed = 0
        let totalSkipped = 0
        for (const file of up.files) {
            const contribution = fileUploadedContribution(file)
            const chain = dirChain(file.dirPath)
            for (const dirRel of chain) {
                const node = up.dirs[dirRel]
                if (!node) continue
                if (file.phase === 'done') node.completed += 1
                if (file.phase === 'error') node.failed += 1
                if (file.phase === 'skipped') node.skipped += 1
                node.loaded += contribution
            }
            if (file.phase === 'done') totalCompleted += 1
            if (file.phase === 'error') totalFailed += 1
            if (file.phase === 'skipped') totalSkipped += 1
            totalLoaded += contribution
        }
        up.totals.completedFiles = totalCompleted
        up.totals.failedFiles = totalFailed
        up.totals.skippedFiles = totalSkipped
        up.totals.loadedBytes = totalLoaded
    }

    function setFilePhase({ index, phase }) {
        const file = uploadProgress.value.files[index]
        if (!file) return
        file.phase = phase
        if (phase === 'uploading') {
            file.loaded = 0
            file.uploadedBytes = 0
        }
        if (phase === 'completing') {
            file.uploadedBytes = file.size
        }
        if (phase === 'done') {
            file.loaded = file.size
            file.uploadedBytes = file.size
        }
        recomputeDirAggregates()
    }

    function setFileProgress({ index, loaded, phase }) {
        const file = uploadProgress.value.files[index]
        if (!file) return
        file.loaded = loaded
        if (phase === 'uploading' && file.phase !== 'completing' && file.phase !== 'done') {
            file.uploadedBytes = Math.min(loaded, file.size)
        }
        recomputeDirAggregates()
    }

    function setFileError({ index, error, detail = null }) {
        const file = uploadProgress.value.files[index]
        if (!file) return
        file.phase = 'error'
        file.error = error
        file.errorDetail = detail
        recomputeDirAggregates()
    }

    function markFilesSkipped(predicate) {
        const up = uploadProgress.value
        for (const file of up.files) {
            if (predicate(file)) {
                file.phase = 'skipped'
            }
        }
        recomputeDirAggregates()
    }

    function resetFailedToPending() {
        const up = uploadProgress.value
        for (const file of up.files) {
            if (file.phase === 'error') {
                file.phase = 'pending'
                file.loaded = 0
                file.uploadedBytes = 0
                file.error = null
                file.errorDetail = null
            }
        }
        recomputeDirAggregates()
    }

    function toggleDirExpanded(dirPath) {
        const node = uploadProgress.value.dirs[dirPath]
        if (node) node.expanded = !node.expanded
    }

    function startArchiveDownload({ filename, abortController, kind }) {
        archiveDownload.value = {
            ...createInitialArchiveDownload(),
            status: 'preparing',
            kind: kind || 'archive',
            phase: 'preparing',
            filename: filename || '',
            abortController: abortController ? markRaw(abortController) : null,
        }
    }

    function setArchivePhase(phase) {
        const ad = archiveDownload.value
        ad.phase = phase
        if (phase === 'downloading') ad.status = 'downloading'
        if (phase === 'completed') ad.status = 'completed'
    }

    function setArchiveProgress({ loaded, total, totalFiles, skippedCount }) {
        const ad = archiveDownload.value
        if (typeof loaded === 'number') ad.loaded = loaded
        if (typeof total === 'number') ad.total = total
        if (typeof totalFiles === 'number') ad.totalFiles = totalFiles
        if (typeof skippedCount === 'number') ad.skippedCount = skippedCount
    }

    function setArchiveError(error) {
        archiveDownload.value = {
            ...archiveDownload.value,
            status: 'error',
            phase: 'error',
            error,
        }
    }

    function clearArchiveDownload() {
        archiveDownload.value = createInitialArchiveDownload()
    }

    function clearUploadProgress() {
        uploadRawFiles.current = []
        uploadListings.current = new Map()
        uploadProgress.value = createInitialUploadProgress()
    }

    function addLoading() {
        loadingCount.value += 1
    }

    function subtractLoading() {
        loadingCount.value -= 1
    }

    function clearLoading() {
        loadingCount.value = 0
    }

    function setError(error) {
        errors.value.push(error)
    }

    function clearErrors() {
        errors.value = []
    }

    return {
        actionResult,
        actionProgress,
        progressLabel,
        loadingCount,
        errors,
        uploadProgress,
        loading,
        setActionResult,
        clearActionResult,
        setProgress,
        clearProgress,
        addLoading,
        subtractLoading,
        clearLoading,
        setError,
        clearErrors,
        initUploadProgress,
        getRawFile,
        setUploadListings,
        getDirListing,
        applyConflicts,
        setUploadStatus,
        setDefaultAction,
        setFileAction,
        setFilePhase,
        setFileProgress,
        setFileError,
        markFilesSkipped,
        resetFailedToPending,
        toggleDirExpanded,
        clearUploadProgress,
        archiveDownload,
        startArchiveDownload,
        setArchivePhase,
        setArchiveProgress,
        setArchiveError,
        clearArchiveDownload,
    }
})
