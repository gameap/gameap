import HTTP from './axios.js';

export default {
    /**
     * Get configuration data from server
     * @returns {*}
     */
    initialize() {
        return HTTP.get('initialize');
    },

    /**
     * Get directories for the tree (upper level)
     * @param disk
     * @param path
     * @returns {*}
     */
    tree(disk, path) {
        return HTTP.get('tree', { params: { disk, path } });
    },

    /**
     * Select disk
     * @param disk
     * @returns {*}
     */
    selectDisk(disk) {
        return HTTP.get('select-disk', { params: { disk } });
    },

    /**
     * Get content (files and folders)
     * @param disk
     * @param path
     * @param config
     * @returns {*}
     */
    content(disk, path, config = {}) {
        return HTTP.get('content', { params: { disk, path }, ...config });
    },

    /**
     * Item properties
     */
    /* properties(disk, path) {
      return HTTP.get('properties', { params: { disk, path } });
    }, */

    /**
     * URL
     * @param disk
     * @param path
     * @returns {*}
     */
    url(disk, path) {
        return HTTP.get('url', { params: { disk, path } });
    },

    /**
     * Get file to editing or showing
     * @param disk
     * @param path
     * @returns {*}
     */
    getFile(disk, path) {
        return HTTP.get('download', {
            responseType: 'text',
            transformResponse: [(data) => data],
            params: { disk, path },
        });
    },

    /**
     * Get file - ArrayBuffer
     * @param disk
     * @param path
     * @returns {*}
     */
    getFileArrayBuffer(disk, path) {
        return HTTP.get('download', {
            responseType: 'arraybuffer',
            params: { disk, path },
        });
    },

    /**
     * Image thumbnail
     * @param disk
     * @param path
     * @returns {*}
     */
    thumbnail(disk, path) {
        return HTTP.get('thumbnails', {
            responseType: 'arraybuffer',
            params: { disk, path },
        });
    },

    /**
     * Image preview
     * @param disk
     * @param path
     * @return {*}
     */
    preview(disk, path) {
        return HTTP.get('preview', {
            responseType: 'arraybuffer',
            params: { disk, path },
        });
    },

    /**
     * Chunked upload session status
     * @param uploadId
     * @returns {Promise<AxiosResponse<any>>}
     */
    uploadSessionStatus(uploadId) {
        return HTTP.get(`upload/sessions/${uploadId}`);
    },
};
