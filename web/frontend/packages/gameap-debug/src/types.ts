// Extend Window interface for global variables
declare global {
    interface Window {
        gameapLang: string
        i18n: Record<string, Record<string, string>>
        Vue: typeof import('vue')
        VueRouter: typeof import('vue-router')
        Pinia: typeof import('pinia')
        axios: MockAxios
    }
}

export interface MockAxios {
    get: <T = unknown>(url: string, config?: unknown) => Promise<{ data: T }>
    post: <T = unknown>(url: string, data?: unknown, config?: unknown) => Promise<{ data: T }>
    put: <T = unknown>(url: string, data?: unknown, config?: unknown) => Promise<{ data: T }>
    delete: <T = unknown>(url: string, config?: unknown) => Promise<{ data: T }>
    patch: <T = unknown>(url: string, data?: unknown, config?: unknown) => Promise<{ data: T }>
}
