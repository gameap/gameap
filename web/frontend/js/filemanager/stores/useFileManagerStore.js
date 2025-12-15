import { defineStore } from 'pinia'
import { ref, computed, reactive } from 'vue'
import GET from '../http/get.js'
import POST from '../http/post.js'
import { useSettingsStore } from './useSettingsStore.js'
import { useMessagesStore } from './useMessagesStore.js'
import { useModalStore } from './useModalStore.js'
import { useTreeStore } from './useTreeStore.js'

function createManagerState() {
    return {
        selectedDisk: null,
        selectedDirectory: null,
        directories: [],
        files: [],
        selected: {
            directories: [],
            files: [],
        },
        sort: {
            field: 'name',
            direction: 'up',
        },
        history: [null],
        historyPointer: 0,
        viewType: 'table',
    }
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
    const disks = ref([])
    const fileCallback = ref(null)
    const fullScreen = ref(false)

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

    function addToSelection(managerName, { type, path }) {
        getManager(managerName).selected[type].push(path)
    }

    function removeFromSelection(managerName, { type, path }) {
        const manager = getManager(managerName)
        const itemIndex = manager.selected[type].indexOf(path)
        if (itemIndex !== -1) {
            manager.selected[type].splice(itemIndex, 1)
        }
    }

    function changeSelected(managerName, { type, path }) {
        const manager = getManager(managerName)
        manager.selected.directories = []
        manager.selected.files = []
        manager.selected[type].push(path)
    }

    function clearSelection(managerName) {
        const manager = getManager(managerName)
        manager.selected.directories = []
        manager.selected.files = []
    }

    function addNewFile(managerName, newFile) {
        getManager(managerName).files.push(newFile)
    }

    function updateFile(managerName, file) {
        const manager = getManager(managerName)
        const itemIndex = manager.files.findIndex((el) => el.basename === file.basename)
        if (itemIndex !== -1) {
            manager.files[itemIndex] = file
        }
    }

    function addNewDirectory(managerName, newDirectory) {
        getManager(managerName).directories.push(newDirectory)
    }

    function setManagerView(managerName, type) {
        getManager(managerName).viewType = type
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
            manager.history.splice(manager.historyPointer + 1, Number.MAX_VALUE)
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

    // Manager actions
    async function selectDirectory(managerName, { path, history }) {
        const settings = useSettingsStore()
        const tree = useTreeStore()
        const manager = getManager(managerName)

        setManagerContent(managerName, { directories: [], files: [] })

        const response = await GET.content(manager.selectedDisk, path)
        if (response.data.result.status === 'success') {
            clearSelection(managerName)
            resetSortSettings(managerName)
            setManagerContent(managerName, response.data)
            setManagerDirectory(managerName, path)

            if (history) {
                addToHistory(managerName, path)
            }

            if (settings.windowsConfig === 2 && path && response.data.directories.length) {
                tree.showSubdirectories(path, manager.selectedDisk)
            }
        }
    }

    async function refreshDirectory(managerName) {
        const manager = getManager(managerName)

        const response = await GET.content(manager.selectedDisk, manager.selectedDirectory)
        clearSelection(managerName)
        resetSortSettings(managerName)
        resetHistory(managerName)

        if (manager.selectedDirectory) {
            addToHistory(managerName, manager.selectedDirectory)
        }

        if (response.data.result.status === 'success') {
            setManagerContent(managerName, response.data)
        } else if (response.data.result.status === 'danger') {
            setManagerDirectory(managerName, null)
            refreshDirectory(managerName)
        }
    }

    function historyBack(managerName) {
        const manager = getManager(managerName)
        selectDirectory(managerName, {
            path: manager.history[manager.historyPointer - 1],
            history: false,
        })
        pointerBack(managerName)
    }

    function historyForward(managerName) {
        const manager = getManager(managerName)
        selectDirectory(managerName, {
            path: manager.history[manager.historyPointer + 1],
            history: false,
        })
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
        const tree = useTreeStore()

        const response = await GET.initialize()
        if (response.data.result.status === 'success') {
            settings.initSettings(response.data.config)
            setDisks(response.data.config.disks)

            let leftDisk = response.data.config.leftDisk
                ? response.data.config.leftDisk
                : diskList.value[0]
            let rightDisk = response.data.config.rightDisk
                ? response.data.config.rightDisk
                : diskList.value[0]
            let leftPath = response.data.config.leftPath
            let rightPath = response.data.config.rightPath

            if (window.location.search) {
                const params = new URLSearchParams(window.location.search)
                if (params.get('leftDisk')) leftDisk = params.get('leftDisk')
                if (params.get('rightDisk')) rightDisk = params.get('rightDisk')
                if (params.get('leftPath')) leftPath = params.get('leftPath')
                if (params.get('rightPath')) rightPath = params.get('rightPath')
            }

            setManagerDisk('left', leftDisk)

            if (leftPath) {
                setManagerDirectory('left', leftPath)
                addToHistory('left', leftPath)
            }

            getLoadContent({ manager: 'left', disk: leftDisk, path: leftPath })

            if (settings.windowsConfig === 3) {
                setManagerDisk('right', rightDisk)
                if (rightPath) {
                    setManagerDirectory('right', rightPath)
                    addToHistory('right', rightPath)
                }
                getLoadContent({ manager: 'right', disk: rightDisk, path: rightPath })
            } else if (settings.windowsConfig === 2) {
                await tree.initTree(leftDisk)
                if (leftPath) {
                    tree.reopenPath(leftPath, leftDisk)
                }
            }
        }
    }

    async function getLoadContent({ manager, disk, path }) {
        const response = await GET.content(disk, path)
        if (response.data.result.status === 'success') {
            setManagerContent(manager, response.data)
        }
    }

    async function selectDiskAction({ disk, manager: managerName }) {
        const settings = useSettingsStore()
        const tree = useTreeStore()

        const response = await GET.selectDisk(disk)
        if (response.data.result.status === 'success') {
            setManagerDisk(managerName, disk)
            resetHistory(managerName)

            if (settings.windowsConfig === 2) {
                tree.initTree(disk)
            }

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

    async function upload({ files, overwrite }) {
        const messages = useMessagesStore()
        const currentDirectory = selectedDirectory.value

        const data = new FormData()
        data.append('disk', selectedDisk.value)
        data.append('path', currentDirectory || '')
        data.append('overwrite', overwrite)
        for (let i = 0; i < files.length; i += 1) {
            data.append('files[]', files[i])
        }

        const config = {
            onUploadProgress(progressEvent) {
                const progress = Math.round((progressEvent.loaded * 100) / progressEvent.total)
                messages.setProgress(progress)
            },
        }

        try {
            const response = await POST.upload(data, config)
            messages.clearProgress()

            if (response.data.result.status === 'success' && currentDirectory === selectedDirectory.value) {
                refreshManagers()
            }
            return response
        } catch {
            messages.clearProgress()
        }
    }

    async function deleteItems(items) {
        const settings = useSettingsStore()
        const tree = useTreeStore()

        const response = await POST.delete({
            disk: selectedDisk.value,
            items,
        })

        if (response.data.result.status === 'success') {
            refreshManagers()

            if (settings.windowsConfig === 2) {
                const onlyDir = items.filter((item) => item.type === 'dir')
                tree.deleteFromTree(onlyDir)
            }
        }
        return response
    }

    async function paste() {
        const settings = useSettingsStore()

        const response = await POST.paste({
            disk: selectedDisk.value,
            path: selectedDirectory.value,
            clipboard: clipboard.value,
        })

        if (response.data.result.status === 'success') {
            refreshAll()

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

    function url({ disk, path }) {
        return GET.url(disk, path)
    }

    async function zip(name) {
        const currentDirectory = selectedDirectory.value
        const manager = getManager(activeManager.value)

        const response = await POST.zip({
            disk: selectedDisk.value,
            path: currentDirectory,
            name,
            elements: manager.selected,
        })

        if (response.data.result.status === 'success' && currentDirectory === selectedDirectory.value) {
            refreshManagers()
        }
        return response
    }

    async function unzip(folder) {
        const currentDirectory = selectedDirectory.value
        const items = getSelectedList(activeManager.value)

        const response = await POST.unzip({
            disk: selectedDisk.value,
            path: items[0].path,
            folder,
        })

        if (response.data.result.status === 'success' && currentDirectory === selectedDirectory.value) {
            refreshAll()
        }
        return response
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
        const settings = useSettingsStore()

        if (settings.windowsConfig === 3) {
            return Promise.all([
                refreshDirectory('left'),
                refreshDirectory('right'),
            ])
        }
        return refreshDirectory('left')
    }

    async function refreshAll() {
        const settings = useSettingsStore()
        const tree = useTreeStore()

        if (settings.windowsConfig === 2) {
            await tree.initTree(left.selectedDisk)
            return Promise.all([
                tree.reopenPath(selectedDirectory.value, left.selectedDisk),
                refreshManagers(),
            ])
        }
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
        const settings = useSettingsStore()
        const tree = useTreeStore()

        if (response.data.result.status === 'success' && oldDir === selectedDirectory.value) {
            if (commitName === 'addNewFile') {
                addNewFile(activeManager.value, response.data[type])
            } else if (commitName === 'updateFile') {
                updateFile(activeManager.value, response.data[type])
            } else if (commitName === 'addNewDirectory') {
                addNewDirectory(activeManager.value, response.data[type])
            }

            repeatSort(activeManager.value)

            if (type === 'directory' && settings.windowsConfig === 2) {
                tree.addToTree({
                    parentPath: oldDir,
                    newDirectory: response.data.tree,
                })
            } else if (
                settings.windowsConfig === 3 &&
                left.selectedDirectory === right.selectedDirectory &&
                left.selectedDisk === right.selectedDisk
            ) {
                if (commitName === 'addNewFile') {
                    addNewFile(inactiveManager.value, response.data[type])
                } else if (commitName === 'updateFile') {
                    updateFile(inactiveManager.value, response.data[type])
                } else if (commitName === 'addNewDirectory') {
                    addNewDirectory(inactiveManager.value, response.data[type])
                }
                repeatSort(inactiveManager.value)
            }
        }
    }

    function resetState() {
        const settings = useSettingsStore()
        const modal = useModalStore()
        const messages = useMessagesStore()
        const tree = useTreeStore()

        // left manager
        setManagerDisk('left', null)
        setManagerDirectory('left', null)
        setManagerContent('left', { directories: [], files: [] })
        clearSelection('left')
        resetSortSettings('left')
        resetHistory('left')
        setManagerView('left', 'table')

        // modals
        modal.clearModal()

        // messages
        messages.clearActionResult()
        messages.clearProgress()
        messages.clearLoading()
        messages.clearErrors()

        if (settings.windowsConfig === 3) {
            // right manager
            setManagerDisk('right', null)
            setManagerDirectory('right', null)
            setManagerContent('right', { directories: [], files: [] })
            clearSelection('right')
            resetSortSettings('right')
            resetHistory('right')
            setManagerView('right', 'table')
        } else if (settings.windowsConfig === 2) {
            // tree
            tree.cleanTree()
            tree.clearTempArray()
        }

        // root state
        activeManager.value = 'left'
        clipboard.value = {
            type: null,
            disk: null,
            directories: [],
            files: [],
        }
        disks.value = []
        fileCallback.value = null
        fullScreen.value = false
    }

    function openPDF({ disk, path }) {
        const win = window.open()

        GET.getFileArrayBuffer(disk, path).then((response) => {
            const blob = new Blob([response.data], { type: 'application/pdf' })
            win.document.write(
                `<iframe src="${URL.createObjectURL(blob)}" allowfullscreen height="100%" width="100%"></iframe>`
            )
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
        // Manager mutations
        setManagerDisk,
        setManagerDirectory,
        setManagerContent,
        addToSelection,
        removeFromSelection,
        changeSelected,
        clearSelection,
        addNewFile,
        updateFile,
        addNewDirectory,
        setManagerView,
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
        delete: deleteItems,
        paste,
        rename,
        url,
        zip,
        unzip,
        toClipboard,
        refreshManagers,
        refreshAll,
        repeatSort,
        updateContent,
        resetState,
        openPDF,
    }
})
