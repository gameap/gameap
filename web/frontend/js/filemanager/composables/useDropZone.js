import { ref, onMounted, onBeforeUnmount } from 'vue'

const SAFE_PATH = /(?:^|\/)\.\.(?:\/|$)/

function sanitizeRelPath(relPath) {
    if (!relPath) return ''
    const normalized = relPath.replace(/\\/g, '/').replace(/^\/+/, '').replace(/\/+/g, '/')
    if (SAFE_PATH.test(normalized)) return null

    return normalized
}

async function readAllEntries(reader) {
    const entries = []
    let batch = await new Promise((resolve, reject) => {
        reader.readEntries(resolve, reject)
    })
    while (batch.length > 0) {
        entries.push(...batch)
        batch = await new Promise((resolve, reject) => {
            reader.readEntries(resolve, reject)
        })
    }

    return entries
}

async function walkEntry(entry, basePath, files, emptyDirs) {
    if (entry.isFile) {
        const file = await new Promise((resolve, reject) => {
            entry.file(resolve, reject)
        })
        const relPath = sanitizeRelPath(basePath ? `${basePath}/${entry.name}` : entry.name)
        if (relPath === null) return
        files.push({
            relPath,
            name: entry.name,
            file,
            size: file.size,
        })

        return
    }

    if (entry.isDirectory) {
        const reader = entry.createReader()
        const children = await readAllEntries(reader)
        const dirRelPath = sanitizeRelPath(basePath ? `${basePath}/${entry.name}` : entry.name)
        if (dirRelPath === null) return
        if (children.length === 0) {
            emptyDirs.push(dirRelPath)

            return
        }
        for (const child of children) {
            await walkEntry(child, dirRelPath, files, emptyDirs)
        }
    }
}

export async function entriesFromDataTransfer(dataTransfer) {
    const files = []
    const emptyDirs = []

    if (dataTransfer.items && dataTransfer.items.length > 0 && typeof dataTransfer.items[0].webkitGetAsEntry === 'function') {
        const entries = []
        for (let i = 0; i < dataTransfer.items.length; i += 1) {
            const item = dataTransfer.items[i]
            if (item.kind !== 'file') continue
            const entry = item.webkitGetAsEntry()
            if (entry) entries.push(entry)
        }
        await Promise.all(entries.map((entry) => walkEntry(entry, '', files, emptyDirs)))

        return { files, emptyDirs }
    }

    if (dataTransfer.files) {
        for (let i = 0; i < dataTransfer.files.length; i += 1) {
            const f = dataTransfer.files[i]
            files.push({ relPath: f.name, name: f.name, file: f, size: f.size })
        }
    }

    return { files, emptyDirs }
}

export function entriesFromFileList(fileList) {
    const files = []
    for (let i = 0; i < fileList.length; i += 1) {
        const f = fileList[i]
        const rawRel = f.webkitRelativePath || f.name
        const relPath = sanitizeRelPath(rawRel)
        if (relPath === null) continue
        files.push({
            relPath,
            name: f.name,
            file: f,
            size: f.size,
        })
    }

    return { files, emptyDirs: [] }
}

// n-upload file infos: a folder dropped on the dragger keeps its structure
// only in fullPath, the File itself has an empty webkitRelativePath.
export function entriesFromUploadFiles(fileInfos) {
    const files = []
    for (const info of fileInfos) {
        if (!info || !info.file) continue
        const relPath = sanitizeRelPath(info.fullPath || info.file.webkitRelativePath || info.file.name)
        if (!relPath) continue
        files.push({
            relPath,
            name: info.file.name,
            file: info.file,
            size: info.file.size,
        })
    }

    return { files, emptyDirs: [] }
}

export function useWindowDropZone({ onDrop, isActive }) {
    const isOver = ref(false)
    let dragCounter = 0

    function hasFiles(event) {
        const types = event.dataTransfer && event.dataTransfer.types
        if (!types) return false
        for (let i = 0; i < types.length; i += 1) {
            if (types[i] === 'Files') return true
        }

        return false
    }

    function onDragEnter(event) {
        if (!isActive()) return
        if (!hasFiles(event)) return
        event.preventDefault()
        dragCounter += 1
        isOver.value = true
    }

    function onDragLeave(event) {
        if (!isActive()) return
        event.preventDefault()
        dragCounter = Math.max(0, dragCounter - 1)
        if (dragCounter === 0) isOver.value = false
    }

    function onDragOver(event) {
        if (!isActive()) return
        if (!hasFiles(event)) return
        event.preventDefault()
        if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy'
    }

    async function onWindowDrop(event) {
        if (!isActive()) {
            isOver.value = false
            dragCounter = 0

            return
        }
        if (!hasFiles(event)) return
        event.preventDefault()
        isOver.value = false
        dragCounter = 0
        const result = await entriesFromDataTransfer(event.dataTransfer)
        if (result.files.length > 0 || result.emptyDirs.length > 0) {
            onDrop(result)
        }
    }

    onMounted(() => {
        window.addEventListener('dragenter', onDragEnter)
        window.addEventListener('dragleave', onDragLeave)
        window.addEventListener('dragover', onDragOver)
        window.addEventListener('drop', onWindowDrop)
    })

    onBeforeUnmount(() => {
        window.removeEventListener('dragenter', onDragEnter)
        window.removeEventListener('dragleave', onDragLeave)
        window.removeEventListener('dragover', onDragOver)
        window.removeEventListener('drop', onWindowDrop)
    })

    return { isOver }
}
