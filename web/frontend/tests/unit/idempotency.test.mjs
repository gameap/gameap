import assert from 'node:assert/strict'
import {readFile} from 'node:fs/promises'
import {test} from 'node:test'

const source = await readFile(new URL('../../js/utils/idempotency.js', import.meta.url), 'utf8')
const {createIdempotentRequest, newIdempotencyKey} = await import(
    `data:text/javascript;base64,${Buffer.from(source).toString('base64')}`
)

test('generates distinct UUIDv4 keys without crypto.randomUUID', () => {
    const keys = new Set(Array.from({length: 100}, newIdempotencyKey))
    assert.equal(keys.size, 100)
    for (const key of keys) {
        assert.match(key, /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
    }
})

for (const status of [undefined, 409, 500, 502, 503, 504]) {
    test(`preserves the key and exact request bytes after ${status ?? 'a network error'}`, () => {
        const pending = createIdempotentRequest()
        const form = {name: 'Original', settings: {z: 1, a: 2}}
        const first = pending.prepare(form)
        pending.settle(status === undefined ? new Error('connection reset') : {response: {status}})

        form.name = 'Edited while waiting'
        form.settings.z = 3
        const retry = pending.prepare(form)
        assert.equal(pending.pending, true)
        assert.deepEqual(retry, first)
        assert.equal(retry.data, '{"name":"Original","settings":{"z":1,"a":2}}')
        assert.equal(retry.headers['Content-Type'], 'application/json')
    })
}

test('request cancellation keeps the original operation pending', () => {
    const pending = createIdempotentRequest()
    const first = pending.prepare({name: 'Original'})
    pending.settle({__CANCEL__: true})
    assert.equal(pending.pending, true)
    assert.deepEqual(pending.prepare(), first)
})

for (const status of [undefined, 400, 422]) {
    test(`allows a new payload after ${status ?? 'success'}`, () => {
        const pending = createIdempotentRequest()
        const first = pending.prepare({name: 'First'})
        pending.settle(status === undefined ? undefined : {response: {status}})
        assert.equal(pending.pending, false)
        const next = pending.prepare({name: 'Second'})
        assert.notEqual(next.headers['Idempotency-Key'], first.headers['Idempotency-Key'])
        assert.equal(next.data, '{"name":"Second"}')
    })
}

test('a cached failure can be abandoned explicitly after checking the result', () => {
    const pending = createIdempotentRequest()
    const first = pending.prepare({name: 'First'})
    pending.settle({response: {status: 500, headers: {'idempotent-replayed': 'true'}}})
    assert.deepEqual(pending.prepare({name: 'Second'}), first)

    pending.reset()
    assert.equal(pending.pending, false)
    const next = pending.prepare({name: 'Second'})
    assert.notEqual(next.headers['Idempotency-Key'], first.headers['Idempotency-Key'])
    assert.equal(next.data, '{"name":"Second"}')
})

test('commands retain an empty body across retries', () => {
    const pending = createIdempotentRequest()
    const first = pending.prepare()
    assert.equal(first.data, undefined)
    assert.equal(first.headers['Content-Type'], undefined)
    pending.settle({response: {status: 409}})
    assert.deepEqual(pending.prepare(), first)
})
