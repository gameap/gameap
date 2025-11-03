import mutations from './mutations.js';
import getters from './getters.js';

// languages
import ru from '../../lang/ru.js';
import en from '../../lang/en.js';
import ar from '../../lang/ar.js';
import sr from '../../lang/sr.js';
import cs from '../../lang/cs.js';
import de from '../../lang/de.js';
import es from '../../lang/es.js';
import nl from '../../lang/nl.js';
/* eslint camelcase: 0 */
import zh_CN from '../../lang/zh_CN.js';
import fa from '../../lang/fa.js';
import it from '../../lang/it.js';
import tr from '../../lang/tr.js';
import fr from '../../lang/fr.js';
import pt_BR from '../../lang/pt_BR.js';
import zh_TW from '../../lang/zh_TW.js';
import pl from '../../lang/pl.js';
import hu from '../../lang/hu.js';

export default {
    namespaced: true,
    state() {
        return {
            // ACL
            acl: null,

            // App version
            version: '4.0.0-dev2',

            // axios headers
            headers: {},

            // axios default URL
            baseUrl: null,

            /**
             * File manager windows configuration
             * 1 - only one file manager window
             * 2 - one file manager window with directories tree module
             * 3 - two file manager windows
             */
            windowsConfig: null,

            // App language
            lang: 'en',

            // Translations (/src/lang)
            translations: {
                ru: Object.freeze(ru),
                en: Object.freeze(en),
                ar: Object.freeze(ar),
                sr: Object.freeze(sr),
                cs: Object.freeze(cs),
                de: Object.freeze(de),
                es: Object.freeze(es),
                nl: Object.freeze(nl),
                'zh-CN': Object.freeze(zh_CN),
                fa: Object.freeze(fa),
                it: Object.freeze(it),
                tr: Object.freeze(tr),
                fr: Object.freeze(fr),
                'pt-BR': Object.freeze(pt_BR),
                'zh-TW': Object.freeze(zh_TW),
                pl: Object.freeze(pl),
                hu: Object.freeze(hu),
            },

            // show or hide hidden files
            hiddenFiles: false,

            // Context menu items
            contextMenu: [
                [
                    {
                        name: 'open',
                        icon: 'fa-regular fa-folder-open',
                    },
                    {
                        name: 'audioPlay',
                        icon: 'fa-regular fa-play',
                    },
                    {
                        name: 'videoPlay',
                        icon: 'fa-regular fa-play',
                    },
                    {
                        name: 'view',
                        icon: 'fa-solid fa-eye',
                    },
                    {
                        name: 'edit',
                        icon: 'fa-solid fa-pen',
                    },
                    {
                        name: 'select',
                        icon: 'fa-solid fa-check',
                    },
                    {
                        name: 'download',
                        icon: 'fa-solid fa-download',
                    },
                ],
                [
                    {
                        name: 'copy',
                        icon: 'fa-regular fa-copy',
                    },
                    {
                        name: 'cut',
                        icon: 'fa-solid fa-scissors',
                    },
                    {
                        name: 'rename',
                        icon: 'fa-regular fa-pen-to-square',
                    },
                    {
                        name: 'paste',
                        icon: 'fa-regular fa-paste',
                    },
                    {
                        name: 'zip',
                        icon: 'fa-solid fa-box-archive',
                    },
                    {
                        name: 'unzip',
                        icon: 'fa-solid fa-box-open',
                    },
                ],
                [
                    {
                        name: 'delete',
                        icon: 'fa-regular fa-trash-can text-danger',
                    },
                ],
                [
                    {
                        name: 'properties',
                        icon: 'fa-regular fa-rectangle-list',
                    },
                ],
            ],

            // Image extensions for view and preview
            imageExtensions: ['png', 'jpg', 'jpeg', 'gif', 'webp'],

            // Image extensions for cropping
            cropExtensions: ['png', 'jpg', 'jpeg', 'webp'],

            // audio extensions for play
            audioExtensions: ['ogg', 'mp3', 'aac', 'wav'],

            // video extensions for play
            videoExtensions: ['webm', 'mp4'],

            // File extensions for code editor
            textExtensions: {
                sh: 'text/x-sh',
                // styles
                css: 'text/css',
                less: 'text/x-less',
                sass: 'text/x-sass',
                scss: 'text/x-scss',
                html: 'text/html',
                // js
                js: 'text/javascript',
                ts: 'text/typescript',
                vue: 'text/x-vue',
                // text
                htaccess: 'text/plain',
                env: 'text/plain',
                txt: 'text/plain',
                log: 'text/plain',
                ini: 'text/x-ini',
                xml: 'application/xml',
                cfg: 'text/plain',
                md: 'text/x-markdown',
                // c-like
                java: 'text/x-java',
                c: 'text/x-csrc',
                cpp: 'text/x-c++src',
                cs: 'text/x-csharp',
                scl: 'text/x-scala',
                php: 'application/x-httpd-php',
                // DB
                sql: 'text/x-sql',
                // other
                pl: 'text/x-perl',
                py: 'text/x-python',
                lua: 'text/x-lua',
                swift: 'text/x-swift',
                rb: 'text/x-ruby',
                go: 'text/x-go',
                yaml: 'text/x-yaml',
                json: 'application/json',
                properties: 'text/plain',
            },
        };
    },
    mutations,
    getters,
};
