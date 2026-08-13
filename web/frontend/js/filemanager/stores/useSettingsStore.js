import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

import ru from '../lang/ru.js'
import en from '../lang/en.js'
import ar from '../lang/ar.js'
import sr from '../lang/sr.js'
import cs from '../lang/cs.js'
import de from '../lang/de.js'
import es from '../lang/es.js'
import nl from '../lang/nl.js'
import zh_CN from '../lang/zh_CN.js'
import fa from '../lang/fa.js'
import it from '../lang/it.js'
import tr from '../lang/tr.js'
import fr from '../lang/fr.js'
import pt_BR from '../lang/pt_BR.js'
import zh_TW from '../lang/zh_TW.js'
import pl from '../lang/pl.js'
import hu from '../lang/hu.js'

export const useSettingsStore = defineStore('fm-settings', () => {
    const acl = ref(null)
    const version = ref('4.0.0-dev2')
    const headers = ref({})
    const baseUrl = ref(null)
    const serverName = ref('')
    const serverId = ref(null)

    /**
     * File manager windows configuration
     * 1 - only one file manager window
     * 2 - one file manager window with directories tree module
     * 3 - two file manager windows
     */
    const windowsConfig = ref(null)

    const lang = ref('en')

    const translations = ref({
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
    })

    const hiddenFiles = ref(false)

    const contextMenu = ref([
        [
            { name: 'open', icon: 'folder-open' },
            { name: 'audioPlay', icon: 'play' },
            { name: 'videoPlay', icon: 'play' },
            { name: 'view', icon: 'search' },
            { name: 'edit', icon: 'edit' },
            { name: 'select', icon: 'file' },
            { name: 'download', icon: 'download' },
            { name: 'downloadDir', icon: 'folder-download' },
            { name: 'zip', icon: 'file-zipper' },
            { name: 'unzip', icon: 'box-open' },
        ],
        [
            { name: 'copy', icon: 'copy' },
            { name: 'cut', icon: 'cut' },
            { name: 'rename', icon: 'edit' },
            { name: 'chmod', icon: 'lock' },
            { name: 'paste', icon: 'paste' },
        ],
        [
            { name: 'delete', icon: 'delete', iconClass: 'text-danger' },
        ],
        [
            { name: 'hash', icon: 'fingerprint' },
            { name: 'properties', icon: 'info' },
        ],
    ])

    const imageExtensions = ref(['png', 'jpg', 'jpeg', 'gif', 'webp'])
    const cropExtensions = ref(['png', 'jpg', 'jpeg', 'webp'])
    const audioExtensions = ref(['ogg', 'mp3', 'aac', 'wav'])
    const videoExtensions = ref(['webm', 'mp4'])

    const textExtensions = ref({
        txt: 'text/plain',
        text: 'text/plain',
        log: 'text/plain',
        out: 'text/plain',
        err: 'text/plain',
        trace: 'text/plain',
        asc: 'text/plain',
        nfo: 'text/plain',
        me: 'text/plain',
        readme: 'text/plain',
        changelog: 'text/plain',
        license: 'text/plain',
        authors: 'text/plain',
        contributors: 'text/plain',
        notice: 'text/plain',
        todo: 'text/plain',
        manifest: 'text/plain',
        lock: 'text/plain',
        sum: 'text/plain',
        mod: 'text/plain',
        work: 'text/plain',

        ini: 'text/x-ini',
        cfg: 'text/plain',
        cnf: 'text/plain',
        conf: 'text/plain',
        config: 'text/plain',
        toml: 'text/x-toml',
        properties: 'text/plain',
        env: 'text/plain',
        htaccess: 'text/plain',
        htpasswd: 'text/plain',
        editorconfig: 'text/plain',
        gitignore: 'text/plain',
        gitattributes: 'text/plain',
        gitmodules: 'text/plain',
        gitconfig: 'text/plain',
        gitkeep: 'text/plain',
        dockerignore: 'text/plain',
        npmignore: 'text/plain',
        npmrc: 'text/plain',
        yarnrc: 'text/plain',
        nvmrc: 'text/plain',
        eslintrc: 'application/json',
        eslintignore: 'text/plain',
        prettierrc: 'application/json',
        prettierignore: 'text/plain',
        babelrc: 'application/json',
        browserslistrc: 'text/plain',
        stylelintrc: 'application/json',
        desktop: 'text/plain',
        service: 'text/plain',
        socket: 'text/plain',
        target: 'text/plain',
        timer: 'text/plain',
        mount: 'text/plain',
        automount: 'text/plain',
        slice: 'text/plain',
        rules: 'text/plain',
        spec: 'text/plain',
        ldif: 'text/plain',
        reg: 'text/plain',

        md: 'text/x-markdown',
        markdown: 'text/x-markdown',
        mdx: 'text/x-markdown',
        mkd: 'text/x-markdown',
        mdown: 'text/x-markdown',
        rst: 'text/x-rst',
        adoc: 'text/x-asciidoc',
        asciidoc: 'text/x-asciidoc',
        tex: 'text/x-stex',
        bib: 'text/x-stex',
        org: 'text/x-org',
        wiki: 'text/plain',
        textile: 'text/x-textile',

        html: 'text/html',
        htm: 'text/html',
        xhtml: 'application/xhtml+xml',
        xht: 'application/xhtml+xml',
        svg: 'image/svg+xml',
        css: 'text/css',
        less: 'text/x-less',
        sass: 'text/x-sass',
        scss: 'text/x-scss',
        styl: 'text/x-styl',
        stylus: 'text/x-styl',
        pcss: 'text/css',
        postcss: 'text/css',

        js: 'text/javascript',
        mjs: 'text/javascript',
        cjs: 'text/javascript',
        jsx: 'text/jsx',
        ts: 'text/typescript',
        tsx: 'text/typescript-jsx',
        cts: 'text/typescript',
        mts: 'text/typescript',
        coffee: 'text/x-coffeescript',
        vue: 'text/x-vue',
        svelte: 'text/x-svelte',
        astro: 'text/x-astro',

        json: 'application/json',
        json5: 'application/json',
        jsonc: 'application/json',
        yaml: 'text/x-yaml',
        yml: 'text/x-yaml',
        xml: 'application/xml',
        xsd: 'application/xml',
        xsl: 'application/xml',
        xslt: 'application/xml',
        plist: 'application/xml',
        csv: 'text/csv',
        tsv: 'text/tab-separated-values',
        psv: 'text/plain',
        tab: 'text/plain',

        sh: 'text/x-sh',
        bash: 'text/x-sh',
        zsh: 'text/x-sh',
        fish: 'text/x-sh',
        ksh: 'text/x-sh',
        csh: 'text/x-sh',
        tcsh: 'text/x-sh',
        ash: 'text/x-sh',
        dash: 'text/x-sh',
        ps1: 'application/x-powershell',
        psm1: 'application/x-powershell',
        psd1: 'application/x-powershell',
        bat: 'application/x-bat',
        cmd: 'application/x-bat',

        c: 'text/x-csrc',
        h: 'text/x-csrc',
        cpp: 'text/x-c++src',
        cc: 'text/x-c++src',
        cxx: 'text/x-c++src',
        'c++': 'text/x-c++src',
        hpp: 'text/x-c++hdr',
        hh: 'text/x-c++hdr',
        hxx: 'text/x-c++hdr',
        'h++': 'text/x-c++hdr',
        inl: 'text/x-c++src',
        ipp: 'text/x-c++src',
        m: 'text/x-objectivec',
        mm: 'text/x-objectivec',
        cs: 'text/x-csharp',
        java: 'text/x-java',
        kt: 'text/x-kotlin',
        kts: 'text/x-kotlin',
        groovy: 'text/x-groovy',
        gradle: 'text/x-groovy',
        clj: 'text/x-clojure',
        cljs: 'text/x-clojure',
        cljc: 'text/x-clojure',
        cljx: 'text/x-clojure',
        edn: 'text/x-clojure',
        scala: 'text/x-scala',
        scl: 'text/x-scala',

        php: 'application/x-httpd-php',
        phtml: 'application/x-httpd-php',
        php3: 'application/x-httpd-php',
        php4: 'application/x-httpd-php',
        php5: 'application/x-httpd-php',
        php7: 'application/x-httpd-php',
        php8: 'application/x-httpd-php',
        phps: 'application/x-httpd-php',
        py: 'text/x-python',
        pyw: 'text/x-python',
        pyi: 'text/x-python',
        pyx: 'text/x-cython',
        rb: 'text/x-ruby',
        rbw: 'text/x-ruby',
        rake: 'text/x-ruby',
        gemspec: 'text/x-ruby',
        pl: 'text/x-perl',
        pm: 'text/x-perl',
        pod: 'text/x-perl',
        t: 'text/x-perl',
        pl6: 'text/x-perl',
        p6: 'text/x-perl',
        raku: 'text/x-perl',
        rakumod: 'text/x-perl',
        lua: 'text/x-lua',
        tcl: 'text/x-tcl',
        swift: 'text/x-swift',
        go: 'text/x-go',
        rs: 'text/x-rustsrc',
        dart: 'application/dart',
        ex: 'text/x-elixir',
        exs: 'text/x-elixir',
        erl: 'text/x-erlang',
        hrl: 'text/x-erlang',
        elm: 'text/x-elm',
        hs: 'text/x-haskell',
        lhs: 'text/x-haskell',
        ml: 'text/x-ocaml',
        mli: 'text/x-ocaml',
        fs: 'text/x-fsharp',
        fsx: 'text/x-fsharp',
        fsi: 'text/x-fsharp',
        nim: 'text/x-nim',
        nims: 'text/x-nim',
        zig: 'text/x-zig',
        cr: 'text/x-crystal',
        d: 'text/x-d',
        v: 'text/x-verilog',
        sv: 'text/x-systemverilog',
        svh: 'text/x-systemverilog',
        vhd: 'text/x-vhdl',
        vhdl: 'text/x-vhdl',
        r: 'text/x-rsrc',
        rmd: 'text/x-markdown',
        jl: 'text/x-julia',
        pas: 'text/x-pascal',
        pp: 'text/x-pascal',
        inc: 'text/plain',
        ada: 'text/x-ada',
        adb: 'text/x-ada',
        ads: 'text/x-ada',
        f: 'text/x-fortran',
        f77: 'text/x-fortran',
        f90: 'text/x-fortran',
        f95: 'text/x-fortran',
        f03: 'text/x-fortran',
        for: 'text/x-fortran',
        ftn: 'text/x-fortran',
        asm: 'text/x-asm',
        s: 'text/x-asm',
        vb: 'text/x-vb',
        vbs: 'text/x-vb',
        vbnet: 'text/x-vb',
        lisp: 'text/x-common-lisp',
        lsp: 'text/x-common-lisp',
        el: 'text/x-emacs-lisp',
        scm: 'text/x-scheme',
        ss: 'text/x-scheme',
        rkt: 'text/x-racket',
        pro: 'text/x-prolog',
        prolog: 'text/x-prolog',

        sql: 'text/x-sql',
        mysql: 'text/x-sql',
        pgsql: 'text/x-sql',
        psql: 'text/x-sql',
        prisma: 'text/x-prisma',
        graphql: 'application/graphql',
        gql: 'application/graphql',

        dockerfile: 'text/x-dockerfile',
        containerfile: 'text/x-dockerfile',
        makefile: 'text/x-makefile',
        mk: 'text/x-makefile',
        mak: 'text/x-makefile',
        cmake: 'text/x-cmake',
        ninja: 'text/plain',
        bazel: 'text/x-starlark',
        bzl: 'text/x-starlark',
        build: 'text/x-starlark',
        tf: 'text/x-terraform',
        tfvars: 'text/x-terraform',
        hcl: 'text/x-hcl',
        nomad: 'text/x-hcl',
        consul: 'text/x-hcl',
        vault: 'text/x-hcl',
        nix: 'text/x-nix',
        pkgbuild: 'text/x-sh',

        tpl: 'text/plain',
        twig: 'text/x-twig',
        mustache: 'text/x-mustache',
        hbs: 'text/x-handlebars',
        handlebars: 'text/x-handlebars',
        ejs: 'text/plain',
        pug: 'text/x-pug',
        jade: 'text/x-pug',
        erb: 'text/x-erb',
        slim: 'text/x-slim',
        haml: 'text/x-haml',
        jinja: 'text/x-jinja',
        jinja2: 'text/x-jinja',
        j2: 'text/x-jinja',
        liquid: 'text/x-liquid',
        njk: 'text/x-jinja',
        volt: 'text/plain',
        latte: 'text/plain',
        blade: 'text/x-blade',

        diff: 'text/x-diff',
        patch: 'text/x-diff',
        rej: 'text/x-diff',

        proto: 'text/x-protobuf',
        thrift: 'text/x-thrift',
        capnp: 'text/plain',
        pb: 'text/x-protobuf',
        avsc: 'application/json',
        avdl: 'text/plain',

        srt: 'text/plain',
        vtt: 'text/vtt',
        sub: 'text/plain',
        lrc: 'text/plain',
        ass: 'text/plain',
        ssa: 'text/plain',
        ics: 'text/calendar',
        vcf: 'text/vcard',

        csproj: 'application/xml',
        vbproj: 'application/xml',
        fsproj: 'application/xml',
        sln: 'text/plain',
        vcproj: 'application/xml',
        vcxproj: 'application/xml',
        pbxproj: 'text/plain',
        xcconfig: 'text/plain',
        entitlements: 'application/xml',
        storyboard: 'application/xml',
        xib: 'application/xml',
        pubxml: 'application/xml',
        props: 'application/xml',
        targets: 'application/xml',
        nuspec: 'application/xml',
        resx: 'application/xml',

        vdf: 'text/plain',
        acf: 'text/plain',
        kv: 'text/plain',
    })

    // Getters
    const authHeader = computed(() => Object.prototype.hasOwnProperty.call(headers.value, 'Authorization'))

    // Actions
    function manualSettings(data) {
        if (Object.prototype.hasOwnProperty.call(data, 'headers')) {
            headers.value = data.headers
        }
        if (Object.prototype.hasOwnProperty.call(data, 'baseUrl')) {
            baseUrl.value = data.baseUrl
        }
        if (Object.prototype.hasOwnProperty.call(data, 'serverName')) {
            serverName.value = data.serverName || ''
        }
        if (Object.prototype.hasOwnProperty.call(data, 'serverId')) {
            serverId.value = data.serverId || null
        }
        if (Object.prototype.hasOwnProperty.call(data, 'windowsConfig')) {
            windowsConfig.value = data.windowsConfig
        }
        if (Object.prototype.hasOwnProperty.call(data, 'lang')) {
            lang.value = data.lang
        }
        if (Object.prototype.hasOwnProperty.call(data, 'translation')) {
            translations.value[data.translation.name] = Object.freeze(data.translation.content)
        }
    }

    function initAxiosSettings() {
        if (!baseUrl.value) {
            if (import.meta.env.VITE_APP_LFM_AXIOS_BASE_URL) {
                baseUrl.value = import.meta.env.VITE_APP_LFM_AXIOS_BASE_URL
            } else if (import.meta.env.VITE_LFM_BASE_URL) {
                baseUrl.value = import.meta.env.VITE_LFM_BASE_URL
            } else {
                let url = `${window.location.protocol}//${window.location.hostname}`
                if (window.location.port.length) {
                    url += `:${window.location.port}/api/file-manager/`
                } else {
                    url += '/api/file-manager/'
                }
                baseUrl.value = url
            }
        }
    }

    function initSettings(data) {
        if (!lang.value) lang.value = data.lang
        if (!windowsConfig.value) windowsConfig.value = data.windowsConfig
        acl.value = data.acl
        hiddenFiles.value = data.hiddenFiles
    }

    function toggleHiddenFiles() {
        hiddenFiles.value = !hiddenFiles.value
    }

    return {
        // State
        acl,
        version,
        headers,
        baseUrl,
        serverName,
        serverId,
        windowsConfig,
        lang,
        translations,
        hiddenFiles,
        contextMenu,
        imageExtensions,
        cropExtensions,
        audioExtensions,
        videoExtensions,
        textExtensions,
        // Getters
        authHeader,
        // Actions
        manualSettings,
        initAxiosSettings,
        initSettings,
        toggleHiddenFiles,
    }
})
