import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import axios from '../config/axios'
import { useServerStore } from './server.js'

export const useServerRconStore = defineStore('serverRcon', () => {
    // State
    const fastRcon = ref([])
    const rconSupportedFeatures = ref({
        rcon: false,
        playersManage: false,
        playersList: false,
        playersKick: false,
        playersBan: false,
    })
    const output = ref('')
    const apiProcesses = ref(0)

    // From legacy rcon/players.js
    const players = ref([])

    // Getters
    const loading = computed(() => apiProcesses.value > 0)

    const serverId = computed(() => {
        const serverStore = useServerStore()
        return serverStore.serverId
    })

    const canUseRcon = computed(() => {
        const serverStore = useServerStore()
        return Boolean(serverStore.abilities['game-server-rcon-console'])
    })

    const canManageRconPlayers = computed(() => {
        const serverStore = useServerStore()
        return Boolean(serverStore.abilities['game-server-rcon-players'])
    })

    // Actions
    function setServerId(id) {
        const serverStore = useServerStore()
        serverStore.setServerId(id)
    }

    async function fetchRconSupportedFeatures() {
        const serverStore = useServerStore()
        apiProcesses.value++

        try {
            const response = await axios.get('/api/servers/' + serverStore.serverId + '/rcon/features')
            rconSupportedFeatures.value = response.data
        } catch (error) {
            if (error.__CANCEL__) {
                return
            }
            rconSupportedFeatures.value = {
                rcon: false,
                playersManage: false,
                playersList: false,
                playersKick: false,
                playersBan: false,
            }
        } finally {
            apiProcesses.value--
        }
    }

    async function fetchFastRcon() {
        const serverStore = useServerStore()
        apiProcesses.value++

        try {
            const response = await axios.get('/api/servers/' + serverStore.serverId + '/rcon/fast_rcon')
            fastRcon.value = response.data
        } finally {
            apiProcesses.value--
        }
    }

    async function sendCommand(command) {
        const serverStore = useServerStore()
        apiProcesses.value++

        try {
            const response = await axios.post('/api/servers/' + serverStore.serverId + '/rcon', {
                command: command
            })
            output.value = response.data.output
        } finally {
            apiProcesses.value--
        }
    }

    // From legacy rcon/players.js
    async function fetchPlayers() {
        const serverStore = useServerStore()
        if (serverStore.serverId <= 0) {
            return
        }

        apiProcesses.value++
        try {
            const response = await axios.get('/api/servers/' + serverStore.serverId + '/rcon/players')
            players.value = response.data
        } finally {
            apiProcesses.value--
        }
    }

    async function kickPlayer(player, reason) {
        const serverStore = useServerStore()

        apiProcesses.value++
        try {
            await axios.post('/api/servers/' + serverStore.serverId + '/rcon/players/kick', {
                player: player,
                reason: reason
            })
            await fetchPlayers()
        } finally {
            apiProcesses.value--
        }
    }

    async function banPlayer(player, reason, time) {
        const serverStore = useServerStore()

        apiProcesses.value++
        try {
            await axios.post('/api/servers/' + serverStore.serverId + '/rcon/players/ban', {
                player: player,
                reason: reason,
                time: time,
            })
            await fetchPlayers()
        } finally {
            apiProcesses.value--
        }
    }

    async function sendPlayerMessage(playerId, message) {
        const serverStore = useServerStore()

        apiProcesses.value++
        try {
            await axios.post('/api/servers/' + serverStore.serverId + '/rcon/players/message', {
                player: playerId,
                message: message,
            })
        } finally {
            apiProcesses.value--
        }
    }

    return {
        // State
        fastRcon,
        rconSupportedFeatures,
        output,
        apiProcesses,
        players,

        // Getters
        loading,
        serverId,
        canUseRcon,
        canManageRconPlayers,

        // Actions
        setServerId,
        fetchRconSupportedFeatures,
        fetchFastRcon,
        sendCommand,
        fetchPlayers,
        kickPlayer,
        banPlayer,
        sendPlayerMessage,
    }
})
