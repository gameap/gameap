export default {
    /**
     * Set config
     * @param state
     * @param data
     */
    manualSettings(state, data) {
        // overwrite headers - Axios
        if (Object.prototype.hasOwnProperty.call(data, 'headers')) {
            state.headers = data.headers;
        }
        // base url - axios
        if (Object.prototype.hasOwnProperty.call(data, 'baseUrl')) {
            state.baseUrl = data.baseUrl;
        }
        // windows config
        if (Object.prototype.hasOwnProperty.call(data, 'windowsConfig')) {
            state.windowsConfig = data.windowsConfig;
        }
        // language
        if (Object.prototype.hasOwnProperty.call(data, 'lang')) {
            state.lang = data.lang;
        }
        // add new translation
        if (Object.prototype.hasOwnProperty.call(data, 'translation')) {
            state.translations[data.translation.name] = Object.freeze(data.translation.content);
        }
    },

    /**
     * Initiate Axios baseUrl and headers
     * @param state
     */
    initAxiosSettings(state) {
        // initiate base url, if not set manually
        if (!state.baseUrl) {
            if (import.meta.env.VITE_APP_LFM_AXIOS_BASE_URL) {
                // vue .env
                state.baseUrl = import.meta.env.VITE_APP_LFM_AXIOS_BASE_URL;
            } else if (import.meta.env.VITE_LFM_BASE_URL) {
                // laravel .env
                state.baseUrl = import.meta.env.VITE_LFM_BASE_URL;
            } else {
                let baseUrl = `${window.location.protocol}//${window.location.hostname}`;

                if (window.location.port.length) {
                    baseUrl += `:${window.location.port}/api/file-manager/`;
                } else {
                    baseUrl += '/api/file-manager/';
                }

                state.baseUrl = baseUrl;
            }
        }
    },

    /**
     * Initialize App settings from server
     * @param state
     * @param data
     */
    initSettings(state, data) {
        if (!state.lang) state.lang = data.lang;
        if (!state.windowsConfig) state.windowsConfig = data.windowsConfig;
        state.acl = data.acl;
        state.hiddenFiles = data.hiddenFiles;
    },

    /**
     * Set Hide or Show hidden files
     * @param state
     */
    toggleHiddenFiles(state) {
        state.hiddenFiles = !state.hiddenFiles;
    },
};
