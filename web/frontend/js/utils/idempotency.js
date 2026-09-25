// newIdempotencyKey returns a random UUIDv4. crypto.randomUUID exists only in
// secure contexts, while a panel is often served over plain http://ip:8025;
// crypto.getRandomValues is available everywhere.
export function newIdempotencyKey() {
    const bytes = crypto.getRandomValues(new Uint8Array(16))
    bytes[6] = (bytes[6] & 0x0f) | 0x40
    bytes[8] = (bytes[8] & 0x3f) | 0x80

    const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')

    return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}

// Keep the exact bytes with their key: a timeout or 5xx can arrive after the
// operation took effect. Only an explicit reset may abandon an unknown outcome.
export function createIdempotentRequest() {
    let key = null
    let body

    return {
        get pending() {
            return key !== null
        },
        prepare(data) {
            if (key === null) {
                body = data === undefined ? undefined : JSON.stringify(data)
                key = newIdempotencyKey()
            }

            return {
                data: body,
                headers: {
                    'Idempotency-Key': key,
                    ...(body === undefined ? {} : {'Content-Type': 'application/json'}),
                },
            }
        },
        settle(error) {
            const status = error?.response?.status

            if (error && (status === undefined || status === 409 || status >= 500)) {
                return
            }

            key = null
            body = undefined
        },
        reset() {
            key = null
            body = undefined
        },
    }
}
