/* eslint-disable no-await-in-loop */
import { createSHA256 } from 'hash-wasm'
import POST from './post.js'
import GET from './get.js'

const HASH_SLICE = 4 * 1024 * 1024
const CHUNK_CONCURRENCY = 4
const CHUNK_RETRIES = 3
const COMPLETE_RETRIES = 3
const RETRY_BASE_DELAY = 300

class UploadError extends Error {
    constructor(code, message, cause) {
        super(message || code)
        this.code = code
        this.cause = cause
    }
}

const sleep = (ms) =>
    new Promise((resolve) => {
        setTimeout(resolve, ms)
    })

const isAborted = (signal) => signal && signal.aborted

const throwIfAborted = (signal) => {
    if (isAborted(signal)) throw new UploadError('aborted', 'Upload aborted')
}

const sumValues = (map) => Array.from(map.values()).reduce((s, v) => s + v, 0)

const classifyAxiosError = (err) => {
    if (!err) return new UploadError('unknown', 'Unknown error')
    if (err instanceof UploadError) return err
    const status = err.response && err.response.status

    if (status === 400) return new UploadError('bad_request', 'Bad request', err)
    if (status === 401) return new UploadError('unauthorized', 'Unauthorized', err)
    if (status === 403) return new UploadError('forbidden', 'Forbidden', err)
    if (status === 404) return new UploadError('session_expired', 'Session not found', err)
    if (status === 410) return new UploadError('session_expired', 'Session expired', err)
    if (status === 409) return new UploadError('conflict', 'Conflict', err)
    if (status === 413) {
        const body = err.response.data
        const fromAPI = body && typeof body === 'object' && body.http_code
        if (!fromAPI) {
            return new UploadError(
                'proxy_limit',
                'Request rejected by reverse proxy (413), check client_max_body_size',
                err,
            )
        }

        return new UploadError('size_mismatch', 'Chunk size mismatch', err)
    }
    if (status === 422) return new UploadError('checksum_mismatch', 'Checksum mismatch', err)
    if (status >= 500 && status < 600) return new UploadError('server_error', `Server ${status}`, err)
    if (err.code === 'ERR_CANCELED') return new UploadError('aborted', 'Aborted', err)
    if (err.message === 'Network Error' || err.code === 'ECONNABORTED') {
        return new UploadError('network', 'Network error', err)
    }

    return new UploadError('unknown', err.message || 'Unknown error', err)
}

const isRetriable = (err) => err.code === 'network' || err.code === 'server_error'

async function hashFile(file, { onProgress, signal }) {
    const hasher = await createSHA256()
    hasher.init()
    let offset = 0
    while (offset < file.size) {
        throwIfAborted(signal)
        const end = Math.min(offset + HASH_SLICE, file.size)
        const buf = new Uint8Array(await file.slice(offset, end).arrayBuffer())
        hasher.update(buf)
        offset = end
        if (onProgress) onProgress({ loaded: offset, total: file.size })
    }

    return hasher.digest('hex')
}

async function putChunkWithRetry({ uploadId, index, blob, signal, onChunkProgress }) {
    let lastErr
    let attempt = 0
    while (attempt < CHUNK_RETRIES) {
        throwIfAborted(signal)
        try {
            await POST.putUploadChunk(uploadId, index, blob, {
                onUploadProgress: (e) => {
                    if (onChunkProgress) onChunkProgress(e.loaded || 0)
                },
                signal,
            })

            return
        } catch (err) {
            const cls = classifyAxiosError(err)
            lastErr = cls
            if (cls.code === 'conflict') return
            if (cls.code === 'aborted') throw cls
            if (!isRetriable(cls)) throw cls
            if (attempt < CHUNK_RETRIES - 1) {
                await sleep(RETRY_BASE_DELAY * 3 ** attempt)
            }
        }
        attempt += 1
    }
    throw lastErr || new UploadError('unknown', 'Chunk upload failed')
}

const chunkSizeAt = (idx, chunkSize, totalChunks, totalSize) =>
    idx === totalChunks - 1 ? totalSize - chunkSize * (totalChunks - 1) : chunkSize

async function runChunkPool({ file, uploadId, indices, chunkSize, totalChunks, totalSize, signal, onProgress }) {
    const queue = indices.slice()
    const inflight = new Map()
    let completedBytes = 0

    const reportProgress = () => {
        if (onProgress) onProgress({ loaded: completedBytes + sumValues(inflight), total: totalSize })
    }

    const worker = async () => {
        while (queue.length > 0) {
            throwIfAborted(signal)
            const idx = queue.shift()
            const start = idx * chunkSize
            const size = chunkSizeAt(idx, chunkSize, totalChunks, totalSize)
            const blob = file.slice(start, start + size)
            inflight.set(idx, 0)
            try {
                await putChunkWithRetry({
                    uploadId,
                    index: idx,
                    blob,
                    signal,
                    onChunkProgress: (loaded) => {
                        inflight.set(idx, loaded)
                        reportProgress()
                    },
                })
                completedBytes += size
            } finally {
                inflight.delete(idx)
            }
            reportProgress()
        }
    }

    const concurrency = Math.min(CHUNK_CONCURRENCY, queue.length || 1)
    await Promise.all(Array.from({ length: concurrency }, () => worker()))
}

async function fetchStatus(uploadId) {
    try {
        const res = await GET.uploadSessionStatus(uploadId)

        return res.data
    } catch (err) {
        throw classifyAxiosError(err)
    }
}

async function completeWithRecovery({ file, session, signal, onProgress }) {
    const { upload_id: uploadId, chunk_size: chunkSize, total_chunks: totalChunks, total_size: totalSize } = session
    let attempt = 0
    let checksumRetried = false

    while (attempt < COMPLETE_RETRIES) {
        throwIfAborted(signal)
        let nextAction = 'complete'
        try {
            const res = await POST.completeUploadSession(uploadId)

            return res.data
        } catch (err) {
            const cls = classifyAxiosError(err)
            if (cls.code === 'conflict') {
                nextAction = 'reupload-missing'
            } else if (cls.code === 'checksum_mismatch' && !checksumRetried) {
                checksumRetried = true
                nextAction = 'reupload-all'
            } else if (cls.code === 'server_error' && attempt < COMPLETE_RETRIES - 1) {
                nextAction = 'backoff'
            } else {
                throw cls
            }
        }

        if (nextAction === 'reupload-missing') {
            const status = await fetchStatus(uploadId)
            if (status.completed) return { upload_id: uploadId, completed: true }
            const missing = status.missing_chunks || []
            if (missing.length > 0) {
                await runChunkPool({
                    file,
                    uploadId,
                    indices: missing,
                    chunkSize,
                    totalChunks,
                    totalSize,
                    signal,
                    onProgress,
                })
            }
        } else if (nextAction === 'reupload-all') {
            await runChunkPool({
                file,
                uploadId,
                indices: Array.from({ length: totalChunks }, (_, i) => i),
                chunkSize,
                totalChunks,
                totalSize,
                signal,
                onProgress,
            })
        } else if (nextAction === 'backoff') {
            await sleep(RETRY_BASE_DELAY * 3 ** attempt)
        }

        attempt += 1
    }

    throw new UploadError('unknown', 'Complete failed after retries')
}

function fireAndForgetAbort(uploadId) {
    if (!uploadId) return
    POST.abortUploadSession(uploadId).catch(() => {})
}

export async function uploadFileChunked(file, { path, filename, onPhase, onProgress, signal } = {}) {
    let createdUploadId = null
    try {
        if (onPhase) onPhase('hashing')
        let expectedChecksum
        try {
            expectedChecksum = await hashFile(file, {
                onProgress: (p) => onProgress && onProgress({ phase: 'hashing', ...p }),
                signal,
            })
        } catch (err) {
            if (err instanceof UploadError) throw err
            throw new UploadError(
                'hash_failed',
                `SHA-256 hashing failed: ${err && err.message ? err.message : err}`,
                err,
            )
        }

        throwIfAborted(signal)
        if (onPhase) onPhase('uploading')
        const createRes = await POST.createUploadSession({
            path: path || '',
            filename: filename || file.name,
            total_size: file.size,
            expected_checksum: expectedChecksum,
        }).catch((err) => {
            throw classifyAxiosError(err)
        })
        const session = createRes.data
        createdUploadId = session.upload_id

        await runChunkPool({
            file,
            uploadId: session.upload_id,
            indices: Array.from({ length: session.total_chunks }, (_, i) => i),
            chunkSize: session.chunk_size,
            totalChunks: session.total_chunks,
            totalSize: session.total_size,
            signal,
            onProgress: (p) => onProgress && onProgress({ phase: 'uploading', ...p }),
        })

        if (onPhase) onPhase('completing')
        const result = await completeWithRecovery({
            file,
            session,
            signal,
            onProgress: (p) => onProgress && onProgress({ phase: 'uploading', ...p }),
        })

        return result
    } catch (err) {
        const cls = err instanceof UploadError ? err : classifyAxiosError(err)
        if (createdUploadId && cls.code !== 'conflict') {
            fireAndForgetAbort(createdUploadId)
        }
        throw cls
    }
}

export { UploadError }
