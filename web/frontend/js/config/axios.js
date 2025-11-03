import axios from 'axios'
import { requestCancellation } from './requestCancellation'

const axiosInstance = axios.create({
    baseURL: import.meta.env.VITE_API_BASE_URL || '',
    withCredentials: true,
})

axiosInstance.interceptors.request.use(
    (config) => {
        const controller = requestCancellation.getCurrentController()
        if (controller && !config.signal) {
            config.signal = controller.signal
        }
        return config
    },
    (error) => {
        return Promise.reject(error)
    }
)

axiosInstance.interceptors.response.use(
    (response) => response,
    (error) => {
        if (axios.isCancel(error)) {
            return Promise.reject({ ...error, __CANCEL__: true })
        }
        return Promise.reject(error)
    }
)

export default axiosInstance