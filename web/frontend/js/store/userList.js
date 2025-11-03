import { defineStore } from 'pinia'
import axios from '../config/axios'

export const useUserListStore = defineStore('userList', {
    state: () => ({
        users: [],

        // This is a counter to keep track of how many API processes are running
        apiProcesses: 0,
    }),
    getters: {
        loading: (state) => state.apiProcesses > 0,
    },
    actions: {
        async fetchUsers() {
            this.apiProcesses++
            try {
                const response = await axios.get('/api/users/')
                this.users = response.data;
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async createUser(user) {
            this.apiProcesses++
            try {
                await axios.post('/api/users', user)
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async deleteUserById(id) {
            this.apiProcesses++
            try {
                await axios.delete('/api/users/'+id)
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        }
    }
})