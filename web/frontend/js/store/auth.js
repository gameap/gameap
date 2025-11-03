import { defineStore } from 'pinia'
import axios from '../config/axios'

export const useAuthStore = defineStore('auth', {
    state: () => ({
        profile: null,
        serversAbilities: {},
        // This is a counter to keep track of how many API processes are running
        apiProcesses: 0,
        authToken: null,
    }),
    getters: {
        loading: (state) => state.apiProcesses > 0,
        isAdmin: (state) => {
            return state.profile && state.profile.roles && state.profile.roles.includes('admin')
        },
        isAuthenticated: (state) => {
            return state.authToken !== null
        },
        user: (state) => {
            return state.profile
        },
        canServerAbility: (state) => (serverId, ability) => {
            if (state.isAdmin) {
                return true
            }

            if (!state.serversAbilities[serverId]) {
                return false
            }

            return state.serversAbilities[serverId][ability]
        }
    },
    actions: {
        async fetchProfile() {
            this.apiProcesses++
            try {
                const response = await axios.get('/api/profile')
                this.profile = response.data
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async saveProfile(profile) {
            this.apiProcesses++
            try {
                await axios.put('/api/profile', profile)
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async fetchServersAbilities() {
            this.apiProcesses++
            try {
                const response = await axios.get('/api/user/servers_abilities')
                this.serversAbilities = response.data
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async login(credentials) {
            this.apiProcesses++
            try {
                const response = await axios.post(
                    '/api/auth/login',
                    credentials,
                    {withCredentials: true},
                )

                // Extract token from response
                const { token, user } = response.data

                // Store token in state
                this.authToken = token

                // Save token to localStorage
                localStorage.setItem('auth_token', token)

                // Set token as default Authorization header for all axios requests
                axios.defaults.headers.common['Authorization'] = `Bearer ${token}`

                // Update user data
                this.profile = user
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async logout() {
            // Clear token from state
            this.authToken = null

            // Remove token from localStorage
            localStorage.removeItem('auth_token')

            // Remove Authorization header
            delete axios.defaults.headers.common['Authorization']

            // Clear user data
            this.profile = null
        },
        async initializeAuth() {
            // Load token from localStorage
            const token = localStorage.getItem('auth_token')

            if (token) {
                // Restore token to state
                this.authToken = token

                // Set token as default Authorization header for all axios requests
                axios.defaults.headers.common['Authorization'] = `Bearer ${token}`
            }

            // Try to get user profile from server-side session/cookies
            try {
                await this.fetchProfile()
            } catch (error) {
                //this.profile = {}

                // Only log if it's not a 401 (unauthorized) error
                if (error.response?.status !== 401
                    && error.response?.status !== 403
                    && error.response?.status !== 404
                ) {
                    console.error('Failed to fetch user profile during auth initialization:', error)

                    if (window.location.pathname !== '/500') {
                        window.location.href = '/500'
                    }

                    return
                }

                // Not authenticated - this is expected for non-logged-in users
                this.profile = null
                this.authToken = null

                // Clear invalid token from localStorage
                localStorage.removeItem('auth_token')

                // Remove Authorization header
                delete axios.defaults.headers.common['Authorization']
            }
        },
    }
})