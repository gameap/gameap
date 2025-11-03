import axios from '@/config/axios';

const newAxios = axios.create({
    baseURL: axios.defaults.baseURL,
    withCredentials: true,
})

newAxios.defaults.headers.common = axios.defaults.headers.common

export default newAxios