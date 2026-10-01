// Files and archives are saved by the browser itself, not streamed through the page, so they go
// straight to disk whatever their size and the connection's certificate. The page only hands the
// browser a URL, authorized by a single-use short-lived token because a navigation cannot carry the
// Authorization header. A hidden frame performs the navigation: a response served as an attachment
// turns into a browser download and never loads into the frame, while an error response is rendered
// into it, which is the one way to tell a failure apart and read its message back.

export class DownloadError extends Error {
    constructor(code, message, cause) {
        super(message || '')
        this.code = code
        this.cause = cause
    }
}

// How long a failed response is still read back. Only a frame that showed an error is removed:
// removing one cancels a request the browser has not turned into a download yet, and an archive
// answers only once its manifest is built, which has no deadline.
const ERROR_WATCH_MS = 10 * 60 * 1000

const codeForStatus = (status) => {
    if (status === 401) return 'unauthorized'
    if (status === 403) return 'forbidden'
    if (status === 404) return 'not_found'
    if (status === 413) return 'too_large'
    if (status === 429) return 'too_many'
    if (status >= 500 && status < 600) return 'server_error'

    return 'unknown'
}

export const requestError = (err) =>
    new DownloadError(
        codeForStatus(err?.response?.status),
        err?.response?.data?.message || err?.message || '',
        err,
    )

const unquote = (literal) => {
    try {
        return JSON.parse(literal)
    } catch {
        return ''
    }
}

// The API answers errors with {"message": ..., "http_code": ...}, but browsers wrap a JSON document
// in viewer markup of their own, so the fields are matched out of the text rather than parsed whole.
// A proxy's HTML error page carries no such fields; its title is the best message there is.
const errorFromDocument = (doc) => {
    const text = (doc.querySelector('pre') || doc.body || doc.documentElement)?.textContent || ''
    const message = text.match(/"message"\s*:\s*("(?:[^"\\]|\\.)*")/)
    const status = text.match(/"http_code"\s*:\s*(\d{3})/)

    return new DownloadError(
        codeForStatus(status ? Number(status[1]) : 0),
        (message ? unquote(message[1]) : '') || (doc.title || '').trim(),
    )
}

const readFrameError = (frame) => {
    const doc = frame.contentDocument
    if (!doc) {
        // A document of another origin, such as the browser's own network-error page, is unreadable.
        return new DownloadError('unknown', '')
    }
    if (doc.URL === 'about:blank') {
        return null
    }

    return errorFromDocument(doc)
}

// openDownloadFrame hands url to the browser. The returned promise resolves with the DownloadError
// read back from an error response, or with null once ERROR_WATCH_MS passes without one.
export function openDownloadFrame(url) {
    return new Promise((resolve) => {
        const frame = document.createElement('iframe')
        frame.hidden = true
        frame.tabIndex = -1
        frame.setAttribute('aria-hidden', 'true')

        let timer = null
        const retire = (error) => {
            clearTimeout(timer)
            frame.remove()
            resolve(error)
        }

        frame.addEventListener('load', () => {
            let error
            try {
                error = readFrameError(frame)
            } catch (err) {
                error = new DownloadError('unknown', '', err)
            }
            if (error) retire(error)
        })

        frame.src = url
        document.body.appendChild(frame)
        timer = setTimeout(() => resolve(null), ERROR_WATCH_MS)
    })
}

const baseUrlToAbsolute = (baseUrl) => {
    const trimmed = String(baseUrl || '').replace(/\/+$/, '')
    if (/^https?:\/\//i.test(trimmed)) {
        return trimmed
    }

    return `${window.location.origin}${trimmed.startsWith('/') ? '' : '/'}${trimmed}`
}

const buildURL = (baseUrl, endpoint, params) => {
    const url = new URL(`${baseUrlToAbsolute(baseUrl)}/${endpoint}`)
    for (const [key, value] of Object.entries(params)) {
        if (value !== undefined && value !== null && value !== '') {
            url.searchParams.set(key, String(value))
        }
    }

    return url.href
}

export const fileDownloadURL = (baseUrl, { disk, path, token }) =>
    buildURL(baseUrl, 'download', { disk, path, token })

export const archiveDownloadURL = (baseUrl, { disk, path, filename, compress, token }) =>
    buildURL(baseUrl, 'download-archive', {
        disk,
        path,
        filename,
        compress: compress > 0 ? compress : undefined,
        token,
    })
