import { defineStore } from 'pinia'
import axios from '../config/axios'

export const useAuthStore = defineStore('auth', {
    state: () => ({
        profile: null,
        serversAbilities: {},
        // This is a counter to keep track of how many API processes are running
        apiProcesses: 0,
        authToken: null,
        // Single-use ticket issued by /api/auth/login when the account has 2FA
        // enabled. Kept in memory only (never localStorage): it is not a
        // session, just a short-lived token to redeem at /api/auth/2fa/verify.
        twoFactorChallengeToken: null,
        // Admin-MFA nudge recommendation returned by login + /api/profile.
        // Drives the "enable 2FA" enforcement modal. Null when no nudge applies.
        mfaNudge: null,
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
        is2FAEnabled: (state) => {
            return !!(state.profile && state.profile.two_factor_enabled)
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
                this.mfaNudge = response.data.mfa_nudge || null
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async saveProfile(profile) {
            this.apiProcesses++
            try {
                const response = await axios.put('/api/profile', profile)

                // A password change revokes every previously-issued session
                // token, including the current one; the response then carries
                // a fresh token that must replace it for the session to
                // survive.
                if (response.data.token) {
                    this.authToken = response.data.token
                    localStorage.setItem('auth_token', response.data.token)
                    axios.defaults.headers.common['Authorization'] = `Bearer ${response.data.token}`
                }
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
        // Exchanges the long-lived token (sent in the Authorization header by
        // the configured axios instance) for a single-use, short-lived token
        // safe to put in a URL — used for WebSocket upgrades where the browser
        // cannot set headers. Intentionally not counted in apiProcesses: it
        // runs before every (re)connect and must not flicker the global
        // loading indicator; the WS status store reflects connection state.
        async fetchShortLivedToken() {
            const response = await axios.post('/api/auth/short-lived-token')

            return response.data.token
        },
        // applyAuthResponse stores the issued session token (state +
        // localStorage + axios default header) and the returned user. Shared
        // by the password login and the 2FA verification step.
        applyAuthResponse(data) {
            const { token, user } = data

            this.authToken = token
            localStorage.setItem('auth_token', token)
            axios.defaults.headers.common['Authorization'] = `Bearer ${token}`
            this.profile = user
        },
        // consumeSsoTicket exchanges a single-use ticket, minted by an
        // external system for one specific user, for a real session. The
        // ticket is not a credential anywhere else: the auth middleware does
        // not recognise its prefix, so it only works here.
        async consumeSsoTicket(ticket) {
            this.apiProcesses++
            try {
                const response = await axios.post(
                    '/api/auth/sso/exchange',
                    {ticket},
                    {withCredentials: true},
                )

                if (response.data.two_factor_required) {
                    this.twoFactorChallengeToken = response.data.challenge_token

                    // Carry the redirect through the challenge so the caller can
                    // honour it once the second factor is verified — otherwise
                    // the panel-requested destination is lost across 2FA.
                    return {
                        twoFactorRequired: true,
                        redirectTo: response.data.redirect_to,
                    }
                }

                this.applyAuthResponse(response.data)

                // An administrator who has not enrolled a second factor yet is
                // let in and asked to, exactly as a password login would. Keeping
                // the nudge here is what raises the enforcement modal over the
                // panel — and, once the grace window closes, the unclosable one
                // over an enrollment-scoped session.
                this.mfaNudge = response.data.mfa_nudge || null

                return {
                    twoFactorRequired: false,
                    redirectTo: response.data.redirect_to,
                    mfaEnrollmentRequired: !!response.data.mfa_enrollment_required,
                }
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

                if (response.data.two_factor_required) {
                    this.twoFactorChallengeToken = response.data.challenge_token

                    return { twoFactorRequired: true }
                }

                this.applyAuthResponse(response.data)
                this.mfaNudge = response.data.mfa_nudge || null

                return {
                    twoFactorRequired: false,
                    mfaEnrollmentRequired: !!response.data.mfa_enrollment_required,
                }
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async verifyTwoFactor(code) {
            this.apiProcesses++
            try {
                const response = await axios.post(
                    '/api/auth/2fa/verify',
                    {
                        challenge_token: this.twoFactorChallengeToken,
                        code,
                    },
                    {withCredentials: true},
                )

                this.applyAuthResponse(response.data)
                this.mfaNudge = response.data.mfa_nudge || null
                this.twoFactorChallengeToken = null
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async setupTwoFactor() {
            this.apiProcesses++
            try {
                const response = await axios.post('/api/profile/2fa/setup')

                return response.data
            } finally {
                this.apiProcesses--
            }
        },
        async confirmTwoFactor(code) {
            this.apiProcesses++
            try {
                const response = await axios.post('/api/profile/2fa/confirm', { code })

                return response.data
            } finally {
                this.apiProcesses--
            }
        },
        async disableTwoFactor(password, code) {
            this.apiProcesses++
            try {
                await axios.delete('/api/profile/2fa', { data: { password, code } })
            } finally {
                this.apiProcesses--
            }
        },
        async regenerateRecoveryCodes(password) {
            this.apiProcesses++
            try {
                const response = await axios.post('/api/profile/2fa/recovery-codes', { password })

                return response.data
            } finally {
                this.apiProcesses--
            }
        },
        // Dismisses the admin-MFA nudge for the snooze window (24h) and stores
        // the recomputed recommendation so the enforcement modal hides.
        async snoozeMfaNudge() {
            const response = await axios.post('/api/profile/2fa/snooze')
            this.mfaNudge = response.data.mfa_nudge || null

            return this.mfaNudge
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
            this.mfaNudge = null
        },
        async initializeAuth() {
            // Load token from localStorage
            const token = localStorage.getItem('auth_token')

            if (!token) {
                return
            }

            // Restore token to state
            this.authToken = token

            // Set token as default Authorization header for all axios requests
            axios.defaults.headers.common['Authorization'] = `Bearer ${token}`

            // Try to get user profile from server
            try {
                await this.fetchProfile()
            } catch (error) {
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