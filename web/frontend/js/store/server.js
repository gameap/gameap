import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import axios from '../config/axios'

export const useServerStore = defineStore('server', () => {
    // State
    const errors = ref([])
    const serverId = ref(0)
    const abilities = ref({
        'game-server-common': false,
        'game-server-start': false,
        'game-server-stop': false,
        'game-server-restart': false,
        'game-server-pause': false,
        'game-server-update': false,
        'game-server-files': false,
        'game-server-tasks': false,
        'game-server-settings': false,
        'game-server-console-view': false,
        'game-server-console-send': false,
        'game-server-rcon-console': false,
        'game-server-rcon-players': false,
        'game-server-metrics': false,
    })
    const server = ref({
        id: 0,
        uuid: '',
        uuid_short: '',
        enabled: false,
        installed: false,
        blocked: false,
        name: '',
        ds_id: 0,
        game_id: 0,
        game_mod_id: 0,
        server_ip: '',
        server_port: 0,
        query_port: 0,
        rcon_port: 0,
        game: {},
        online: false,
        rcon: '',
        dir: '',
        su_user: '',
        start_command: '',
        aliases: null,
        cpu_limit: null,
        ram_limit: null,
        vars: null,
    })
    const settings = ref([])
    const apiProcesses = ref(0)

    // From legacy servers.js - form port state
    const formIp = ref('')
    const formPort = ref(0)
    const formQueryPort = ref(0)
    const formRconPort = ref(0)

    // Getters
    const loading = computed(() => apiProcesses.value > 0)
    const canStart = computed(() => Boolean(abilities.value['game-server-start']))
    const canStop = computed(() => Boolean(abilities.value['game-server-stop']))
    const canRestart = computed(() => Boolean(abilities.value['game-server-restart']))
    const canUpdate = computed(() => Boolean(abilities.value['game-server-update']))
    const canReadConsole = computed(() => Boolean(abilities.value['game-server-console-view']))
    const canSendConsole = computed(() => Boolean(abilities.value['game-server-console-send']))
    const canManageFiles = computed(() => Boolean(abilities.value['game-server-files']))
    const canManageTasks = computed(() => Boolean(abilities.value['game-server-tasks']))
    const canManageSettings = computed(() => Boolean(abilities.value['game-server-settings']))
    const canViewMetrics = computed(() => Boolean(abilities.value['game-server-metrics']))
    const getServer = computed(() => server.value)

    // Actions
    function setServerId(id) {
        serverId.value = id
    }

    async function fetchServer() {
        apiProcesses.value++
        try {
            const response = await axios.get('/api/servers/' + serverId.value)
            server.value = response.data
        } catch (error) {
            if (error.__CANCEL__) {
                return
            }
            throw error
        } finally {
            apiProcesses.value--
        }
    }

    async function fetchAbilities() {
        apiProcesses.value++
        try {
            const response = await axios.get('/api/servers/' + serverId.value + '/abilities')
            abilities.value = response.data
        } catch (error) {
            if (error.__CANCEL__) {
                return
            }
            throw error
        } finally {
            apiProcesses.value--
        }
    }

    async function fetchSettings() {
        apiProcesses.value++
        try {
            const response = await axios.get('/api/servers/' + serverId.value + '/settings')
            settings.value = response.data
        } catch (error) {
            if (error.__CANCEL__) {
                return
            }
            throw error
        } finally {
            apiProcesses.value--
        }
    }

    async function save(serverData) {
        apiProcesses.value++
        try {
            await axios.put('/api/servers/' + serverId.value, serverData)
        } catch (error) {
            if (error.__CANCEL__) {
                return
            }
            throw error
        } finally {
            apiProcesses.value--
        }
    }

    async function saveSettings(settingsData) {
        apiProcesses.value++
        try {
            await axios.put('/api/servers/' + serverId.value + '/settings', settingsData)
        } catch (error) {
            if (error.__CANCEL__) {
                return
            }
            throw error
        } finally {
            apiProcesses.value--
        }
    }

    // From legacy servers.js - form port setters
    function setFormIp(ip) {
        formIp.value = ip
    }

    function setFormPort(port) {
        formPort.value = port
    }

    function setFormQueryPort(port) {
        formQueryPort.value = port
    }

    function setFormRconPort(port) {
        formRconPort.value = port
    }

    return {
        // State
        errors,
        serverId,
        abilities,
        server,
        settings,
        apiProcesses,
        formIp,
        formPort,
        formQueryPort,
        formRconPort,

        // Getters
        loading,
        canStart,
        canStop,
        canRestart,
        canUpdate,
        canReadConsole,
        canSendConsole,
        canManageFiles,
        canManageTasks,
        canManageSettings,
        canViewMetrics,
        getServer,

        // Actions
        setServerId,
        fetchServer,
        fetchAbilities,
        fetchSettings,
        save,
        saveSettings,
        setFormIp,
        setFormPort,
        setFormQueryPort,
        setFormRconPort,
    }
})
