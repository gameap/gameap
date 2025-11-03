import { defineStore } from 'pinia'
import axios from '../config/axios'

export const useClientCertificatesStore = defineStore('clientCertificates', {
    state: () => ({
        certificates: [],

        apiProcesses: 0,
    }),
    getters: {
        loading: (state) => state.apiProcesses > 0,
    },
    actions: {
        async fetchClientCertificates() {
            this.apiProcesses++
            try {
                const response = await axios.get('/api/client_certificates')
                this.certificates = response.data;
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async createClientCertificate(certificate) {
            this.apiProcesses++

            var formData = new FormData()
            formData.append("certificate", certificate.certificate)
            formData.append("private_key", certificate.private_key)
            formData.append("private_key_pass", certificate.private_key_pass)

            try {
                await axios.post('/api/client_certificates', formData, {
                    headers: {
                        'Content-Type': 'multipart/form-data'
                    }
                })
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async deleteClientCertificate(certificateId) {
            this.apiProcesses++
            try {
                await axios.delete('/api/client_certificates/'+certificateId)
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
    },
})