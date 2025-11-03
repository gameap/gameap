// Pinia store. Vuex should be replaced with Pinia in the future.
import { defineStore } from 'pinia'

import axios from '../config/axios'

export const useServerStore = defineStore('server', {
    state: () => ({
        errors: [],
        serverId: 0,
        abilities: {
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
        },
        server: {
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

            // admin
            rcon: '',
            dir: '',
            su_user: '',
            start_command: '',
            aliases: null,
        },
        settings: [],

        apiProcesses: 0,
    }),
    getters: {
        loading: (state) => state.apiProcesses > 0,
        canStart(state) {
            return Boolean(state.abilities['game-server-start'])
        },
        canStop(state) {
            return Boolean(state.abilities['game-server-stop'])
        },
        canRestart(state) {
            return Boolean(state.abilities['game-server-restart'])
        },
        canUpdate(state) {
            return Boolean(state.abilities['game-server-update'])
        },
        canReadConsole(state) {
            return Boolean(state.abilities['game-server-console-view'])
        },
        canSendConsole(state) {
            return Boolean(state.abilities['game-server-console-send'])
        },
        canManageFiles(state) {
            return Boolean(state.abilities['game-server-files'])
        },
        canManageTasks(state) {
            return Boolean(state.abilities['game-server-tasks'])
        },
        canManageSettings(state) {
            return Boolean(state.abilities['game-server-settings'])
        },
        getServer(state) {
            return state.server;
        }
    },
    actions: {
        setServerId(serverId) {
            this.serverId = serverId;
        },
        async fetchServer() {
            this.apiProcesses++

            try {
                const response = await axios.get('/api/servers/' + this.serverId)
                this.server = response.data;
            } catch (error) {
                if (error.__CANCEL__) {
                    return
                }
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async fetchAbilities() {
            this.apiProcesses++

            try {
                const response = await axios.get('/api/servers/' + this.serverId + '/abilities')
                this.abilities = response.data;
            } catch (error) {
                if (error.__CANCEL__) {
                    return
                }
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async fetchSettings() {
            this.apiProcesses++

            try {
                const response = await axios.get('/api/servers/' + this.serverId + '/settings')
                this.settings = response.data;
            } catch (error) {
                if (error.__CANCEL__) {
                    return
                }
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async save(server) {
            this.apiProcesses++

            try {
                await axios.put('/api/servers/' + this.serverId, server)
            } catch (error) {
                if (error.__CANCEL__) {
                    return
                }
                throw error
            } finally {
                this.apiProcesses--
            }

        },
        async saveSettings(settings) {
            this.apiProcesses++

            try {
                await axios.put('/api/servers/' + this.serverId + '/settings', settings)
            } catch (error) {
                if (error.__CANCEL__) {
                    return
                }
                throw error
            } finally {
                this.apiProcesses--
            }
        }
    },
})