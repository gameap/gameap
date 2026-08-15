import streamSaver from 'streamsaver'

streamSaver.mitm = '/streamsaver/mitm.html?version=2.0.0'

export class StreamDownloadError extends Error {
    constructor(code, message, cause) {
        super(message || code)
        this.code = code
        this.cause = cause
    }
}

export const parseIntHeader = (response, name) => {
    const value = response.headers.get(name)
    if (!value) return 0
    const num = parseInt(value, 10)

    return Number.isFinite(num) ? num : 0
}

const extractMessageFromBody = (raw) => {
    if (!raw) return ''
    const trimmed = raw.trim()
    if (!trimmed) return ''

    if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
        try {
            const parsed = JSON.parse(trimmed)
            const candidate = parsed?.message || parsed?.error || parsed?.detail
            if (typeof candidate === 'string' && candidate.trim()) {
                return candidate.trim()
            }
            if (Array.isArray(parsed?.errors) && parsed.errors.length > 0) {
                const first = parsed.errors[0]
                if (typeof first === 'string') return first
                if (first?.message) return String(first.message)
            }
        } catch {
            /* not JSON, fall through to raw text */
        }
    }

    return trimmed
}

export const classifyResponseError = async (response) => {
    let raw = ''
    try {
        raw = await response.text()
    } catch {
        raw = ''
    }
    const message = extractMessageFromBody(raw)

    const status = response.status
    if (status === 401) return new StreamDownloadError('unauthorized', message || 'Unauthorized')
    if (status === 403) return new StreamDownloadError('forbidden', message || 'Forbidden')
    if (status === 404) return new StreamDownloadError('not_found', message || 'Not found')
    if (status === 413) return new StreamDownloadError('too_large', message || 'Payload too large')
    if (status === 429) return new StreamDownloadError('too_many', message || 'Too many concurrent downloads')
    if (status >= 500 && status < 600) {
        return new StreamDownloadError('server_error', message || `Server error ${status}`)
    }

    return new StreamDownloadError('unknown', message || `Status ${status}`)
}

export const baseUrlToAbsolute = (baseUrl) => {
    const trimmed = String(baseUrl || '').replace(/\/+$/, '')
    if (/^https?:\/\//i.test(trimmed)) {
        return trimmed
    }

    return `${window.location.origin}${trimmed.startsWith('/') ? '' : '/'}${trimmed}`
}

export const buildAuthHeaders = () => {
    const headers = {}
    const token = localStorage.getItem('auth_token')
    if (token) {
        headers.Authorization = `Bearer ${token}`
    }

    return headers
}

export async function streamSaveResponse({
    url,
    filename,
    headers: extraHeaders,
    sizeHeader,
    signal,
    onResponse,
    onProgress,
}) {
    let response
    try {
        response = await fetch(url, {
            method: 'GET',
            credentials: 'include',
            headers: { ...buildAuthHeaders(), ...(extraHeaders || {}) },
            signal,
        })
    } catch (err) {
        if (signal && signal.aborted) {
            throw new StreamDownloadError('aborted', 'Aborted', err)
        }
        throw new StreamDownloadError('network', err.message || 'Network error', err)
    }

    if (!response.ok) {
        throw await classifyResponseError(response)
    }

    if (!response.body) {
        throw new StreamDownloadError('unsupported', 'ReadableStream is not supported in this browser')
    }

    const total = sizeHeader ? parseIntHeader(response, sizeHeader) : 0
    const contentLength = parseIntHeader(response, 'Content-Length')
    const effectiveTotal = total || contentLength

    if (typeof onResponse === 'function') {
        onResponse(response, { total: effectiveTotal })
    }

    if (typeof onProgress === 'function') {
        onProgress({ loaded: 0, total: effectiveTotal })
    }

    if (signal && signal.aborted) {
        throw new StreamDownloadError('aborted', 'Aborted')
    }

    if (!window.isSecureContext) {
        return blobSaveResponse({ response, filename, effectiveTotal, signal, onProgress })
    }

    // Size hints such as X-Archive-Total-Bytes describe the uncompressed payload, not the stream,
    // so only a real Content-Length may become the browser download's Content-Length.
    const writable = streamSaver.createWriteStream(
        filename,
        contentLength > 0 ? { size: contentLength } : undefined,
    )

    let loaded = 0
    const progressTransform = new TransformStream({
        transform(chunk, controller) {
            loaded += chunk.byteLength
            if (typeof onProgress === 'function') {
                onProgress({ loaded, total: effectiveTotal })
            }
            controller.enqueue(chunk)
        },
    })

    // pipeTo owns cancellation: on abort it aborts the StreamSaver writable (the browser then
    // discards the download as cancelled) and cancels the response body.
    try {
        await response.body.pipeThrough(progressTransform).pipeTo(writable, { signal })
    } catch (err) {
        if (signal && signal.aborted) {
            throw new StreamDownloadError('aborted', 'Aborted', err)
        }
        throw new StreamDownloadError('stream', err.message || 'Stream error', err)
    }

    return { total: effectiveTotal, response }
}

async function blobSaveResponse({ response, filename, effectiveTotal, signal, onProgress }) {
    const reader = response.body.getReader()
    const chunks = []
    let loaded = 0

    // Cancelling the reader unblocks a pending read() with done=true instead of an error,
    // so the aborted check below the loop is what keeps a partial file from being saved.
    const onAbort = () => {
        reader.cancel('aborted by user').catch(() => {
            /* the fetch abort may already have errored the stream */
        })
    }
    if (signal) {
        signal.addEventListener('abort', onAbort, { once: true })
    }

    try {
        for (;;) {
            const { done, value } = await reader.read()
            if (done) break
            chunks.push(value)
            loaded += value.byteLength
            if (typeof onProgress === 'function') {
                onProgress({ loaded, total: effectiveTotal })
            }
        }
    } catch (err) {
        if (signal && signal.aborted) {
            throw new StreamDownloadError('aborted', 'Aborted', err)
        }
        throw new StreamDownloadError('stream', err.message || 'Stream error', err)
    } finally {
        if (signal) {
            signal.removeEventListener('abort', onAbort)
        }
    }

    if (signal && signal.aborted) {
        throw new StreamDownloadError('aborted', 'Aborted')
    }

    const contentType = response.headers.get('Content-Type') || 'application/octet-stream'
    const blob = new Blob(chunks, { type: contentType })
    const blobUrl = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = blobUrl
    anchor.download = filename
    anchor.style.display = 'none'
    document.body.appendChild(anchor)
    anchor.click()
    document.body.removeChild(anchor)
    setTimeout(() => URL.revokeObjectURL(blobUrl), 60000)

    return { total: effectiveTotal, response }
}
