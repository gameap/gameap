import HTTP from './axios.js';

export default {
    /**
     * Create new file
     * @param disk
     * @param path
     * @param name
     * @returns {Promise<AxiosResponse<any>>}
     */
    createFile(disk, path, name) {
        return HTTP.post('create-file', { disk, path, name });
    },

    /**
     * Update file
     * @param formData
     * @returns {*}
     */
    updateFile(formData) {
        return HTTP.post('update-file', formData);
    },

    /**
     * Create new directory
     * @param data
     * @param config
     * @returns {*}
     */
    createDirectory(data, config = {}) {
        return HTTP.post('create-directory', data, config);
    },

    /**
     * Upload file (legacy single-request endpoint)
     * @param data
     * @param config
     * @returns {Promise<AxiosResponse<any>>}
     */
    upload(data, config) {
        return HTTP.post('upload', data, config);
    },

    /**
     * Create chunked upload session
     * @param body
     * @returns {Promise<AxiosResponse<any>>}
     */
    createUploadSession(body) {
        return HTTP.post('upload/sessions', body);
    },

    /**
     * Upload one chunk by zero-based index
     * @param uploadId
     * @param index
     * @param blob
     * @param config
     * @returns {Promise<AxiosResponse<any>>}
     */
    putUploadChunk(uploadId, index, blob, config = {}) {
        return HTTP.put(`upload/sessions/${uploadId}/chunks/${index}`, blob, {
            ...config,
            headers: {
                ...(config.headers || {}),
                'Content-Type': 'application/octet-stream',
            },
        });
    },

    /**
     * Finalize chunked upload
     * @param uploadId
     * @returns {Promise<AxiosResponse<any>>}
     */
    completeUploadSession(uploadId) {
        return HTTP.post(`upload/sessions/${uploadId}/complete`);
    },

    /**
     * Abort chunked upload session
     * @param uploadId
     * @returns {Promise<AxiosResponse<any>>}
     */
    abortUploadSession(uploadId) {
        return HTTP.delete(`upload/sessions/${uploadId}`);
    },

    /**
     * Delete selected items
     * @param data
     * @returns {*}
     */
    delete(data) {
        return HTTP.post('delete', data);
    },

    /**
     * Rename file or folder
     * @param data
     * @returns {*}
     */
    rename(data) {
        return HTTP.post('rename', data);
    },

    /**
     * Copy / Cut files and folders
     * @param data
     * @returns {*}
     */
    paste(data) {
        return HTTP.post('paste', data);
    },

    /**
     * Change file permissions
     * @param data
     * @returns {*}
     */
    chmod(data) {
        return HTTP.post('chmod', data);
    },

    /**
     * Compute file checksums (synchronous)
     * @param data
     * @param config
     * @returns {*}
     */
    hash(data, config = {}) {
        return HTTP.post('hash', data, config);
    },

    /**
     * Start server-side archive creation, resolves to 202 {operation_id}
     * @param data
     * @param config
     * @returns {*}
     */
    archive(data, config = {}) {
        return HTTP.post('archive', data, config);
    },

    /**
     * Start server-side archive extraction, resolves to 202 {operation_id}
     * @param data
     * @param config
     * @returns {*}
     */
    extract(data, config = {}) {
        return HTTP.post('extract', data, config);
    },

    /**
     * Request cancellation of a running archive operation
     * @param operationId
     * @param config
     * @returns {*}
     */
    cancelArchiveOperation(operationId, config = {}) {
        return HTTP.post(`archive-operations/${operationId}/cancel`, null, config);
    },
};
