export default {
    methods: {
        /**
         * Bytes to KB, MB, ..
         * @param bytes
         * @returns {string}
         */
        bytesToHuman(bytes) {
            const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];

            if (bytes === 0) return '0 Bytes';

            const i = parseInt(Math.floor(Math.log(bytes) / Math.log(1024)), 10);

            if (i === 0) return `${bytes} ${sizes[i]}`;

            return `${(bytes / 1024 ** i).toFixed(1)} ${sizes[i]}`;
        },

        /**
         * Timestamp to date
         * @param timestamp
         * @returns {string}
         */
        timestampToDate(timestamp) {
            // if date not defined
            if (timestamp === undefined || timestamp === null) return '-';

            const date = new Date(timestamp * 1000);

            return date.toLocaleString(this.$store.state.fm.settings.lang);
        },

        /**
         * Mime type to icon
         * @param mime
         * @returns {*}
         */
        mimeToIcon(mime) {
            // mime types
            const mimeTypes = {
                // image
                'image/gif': 'fa-regular fa-file-image',
                'image/png': 'fa-regular fa-file-image',
                'image/jpeg': 'fa-regular fa-file-image',
                'image/bmp': 'fa-regular fa-file-image',
                'image/webp': 'fa-regular fa-file-image',
                'image/tiff': 'fa-regular fa-file-image',
                'image/svg+xml': 'fa-regular fa-file-image',

                // text
                'text/plain': 'fa-regular fa-file-lines',

                // code
                'text/javascript': 'fa-regular fa-file-code',
                'application/json': 'fa-regular fa-file-code',
                'text/markdown': 'fa-regular fa-file-code',
                'text/html': 'fa-regular fa-file-code',
                'text/css': 'fa-regular fa-file-code',

                // audio
                'audio/midi': 'fa-regular fa-file-audio',
                'audio/mpeg': 'fa-regular fa-file-audio',
                'audio/webm': 'fa-regular fa-file-audio',
                'audio/ogg': 'fa-regular fa-file-audio',
                'audio/wav': 'fa-regular fa-file-audio',
                'audio/aac': 'fa-regular fa-file-audio',
                'audio/x-wav': 'fa-regular fa-file-audio',
                'audio/mp4': 'fa-regular fa-file-audio',

                // video
                'video/webm': 'fa-regular fa-file-video',
                'video/ogg': 'fa-regular fa-file-video',
                'video/mpeg': 'fa-regular fa-file-video',
                'video/3gpp': 'fa-regular fa-file-video',
                'video/x-flv': 'fa-regular fa-file-video',
                'video/mp4': 'fa-regular fa-file-video',
                'video/quicktime': 'fa-regular fa-file-video',
                'video/x-msvideo': 'fa-regular fa-file-video',
                'video/vnd.dlna.mpeg-tts': 'fa-regular fa-file-video',

                // archive
                'application/x-bzip': 'fa-solid fa-file-zipper',
                'application/x-bzip2': 'fa-solid fa-file-zipper',
                'application/x-tar': 'fa-solid fa-file-zipper',
                'application/gzip': 'fa-solid fa-file-zipper',
                'application/zip': 'fa-solid fa-file-zipper',
                'application/x-7z-compressed': 'fa-solid fa-file-zipper',
                'application/x-rar-compressed': 'fa-solid fa-file-zipper',

                // application
                'application/pdf': 'fa-regular fa-file-pdf',
                'application/rtf': 'fa-regular fa-file-word',
                'application/msword': 'fa-regular fa-file-word',

                'application/vnd.ms-word': 'fa-regular fa-file-word',
                'application/vnd.ms-excel': 'fa-regular fa-file-excel',
                'application/vnd.ms-powerpoint': 'fa-solid fa-file-powerpoint',

                'application/vnd.oasis.opendocument.text': 'fa-regular fa-file-word',
                'application/vnd.oasis.opendocument.spreadsheet': 'fa-regular fa-file-excel',
                'application/vnd.oasis.opendocument.presentation': 'fa-solid fa-file-powerpoint',

                'application/vnd.openxmlformats-officedocument.wordprocessingml': 'fa-regular fa-file-word',
                'application/vnd.openxmlformats-officedocument.spreadsheetml': 'fa-regular fa-file-excel',
                'application/vnd.openxmlformats-officedocument.presentationml': 'fa-solid fa-file-powerpoint',
            };

            if (mimeTypes[mime] !== undefined) {
                return mimeTypes[mime];
            }

            // file blank
            return 'fa-regular fa-file';
        },

        /**
         * File extension to icon (font awesome)
         * @returns {*}
         * @param extension
         */
        extensionToIcon(extension) {
            // files extensions
            const extensionTypes = {
                // images
                gif: 'fa-regular fa-file-image',
                png: 'fa-regular fa-file-image',
                jpeg: 'fa-regular fa-file-image',
                jpg: 'fa-regular fa-file-image',
                bmp: 'fa-regular fa-file-image',
                psd: 'fa-regular fa-file-image',
                svg: 'fa-regular fa-file-image',
                ico: 'fa-regular fa-file-image',
                ai: 'fa-regular fa-file-image',
                tif: 'fa-regular fa-file-image',
                tiff: 'fa-regular fa-file-image',
                webp: 'fa-regular fa-file-image',

                // text
                txt: 'fa-regular fa-file-lines',
                json: 'fa-regular fa-file-lines',
                log: 'fa-regular fa-file-lines',
                ini: 'fa-regular fa-file-lines',
                xml: 'fa-regular fa-file-lines',
                md: 'fa-regular fa-file-lines',
                env: 'fa-regular fa-file-lines',

                // code
                js: 'fa-regular fa-file-code',
                php: 'fa-regular fa-file-code',
                css: 'fa-regular fa-file-code',
                cpp: 'fa-regular fa-file-code',
                class: 'fa-regular fa-file-code',
                h: 'fa-regular fa-file-code',
                java: 'fa-regular fa-file-code',
                sh: 'fa-regular fa-file-code',
                swift: 'fa-regular fa-file-code',

                // audio
                aif: 'fa-regular fa-file-audio',
                cda: 'fa-regular fa-file-audio',
                mid: 'fa-regular fa-file-audio',
                mp3: 'fa-regular fa-file-audio',
                mpa: 'fa-regular fa-file-audio',
                ogg: 'fa-regular fa-file-audio',
                wav: 'fa-regular fa-file-audio',
                wma: 'fa-regular fa-file-audio',

                // video
                wmv: 'fa-regular fa-file-video',
                avi: 'fa-regular fa-file-video',
                mpeg: 'fa-regular fa-file-video',
                mpg: 'fa-regular fa-file-video',
                flv: 'fa-regular fa-file-video',
                mp4: 'fa-regular fa-file-video',
                mkv: 'fa-regular fa-file-video',
                mov: 'fa-regular fa-file-video',
                ts: 'fa-regular fa-file-video',
                '3gpp': 'fa-regular fa-file-video',

                // archive
                zip: 'fa-solid fa-file-zipper',
                arj: 'fa-solid fa-file-zipper',
                deb: 'fa-solid fa-file-zipper',
                pkg: 'fa-solid fa-file-zipper',
                rar: 'fa-solid fa-file-zipper',
                rpm: 'fa-solid fa-file-zipper',
                '7z': 'fa-solid fa-file-zipper',
                'tar.gz': 'fa-solid fa-file-zipper',

                // application
                pdf: 'fa-regular fa-file-pdf',

                rtf: 'fa-regular fa-file-word',
                doc: 'fa-regular fa-file-word',
                docx: 'fa-regular fa-file-word',
                odt: 'fa-regular fa-file-word',

                xlr: 'fa-regular fa-file-excel',
                xls: 'fa-regular fa-file-excel',
                xlsx: 'fa-regular fa-file-excel',

                ppt: 'fa-solid fa-file-powerpoint',
                pptx: 'fa-solid fa-file-powerpoint',
                pptm: 'fa-solid fa-file-powerpoint',
                xps: 'fa-solid fa-file-powerpoint',
                potx: 'fa-solid fa-file-powerpoint',
            };

            if (extension && extensionTypes[extension.toLowerCase()] !== undefined) {
                return extensionTypes[extension.toLowerCase()];
            }

            // blank file
            return 'fa-regular fa-file';
        },
    },
};
