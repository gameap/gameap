import assert from 'node:assert/strict'
import {readFile} from 'node:fs/promises'
import {test} from 'node:test'

const source = await readFile(new URL('../../js/parts/portRange.js', import.meta.url), 'utf8')
const {findFreePort, inPortRange, parsePortRange} = await import(
    `data:text/javascript;base64,${Buffer.from(source).toString('base64')}`
)

test('parses ports and ranges into sorted merged spans', () => {
    assert.deepEqual(parsePortRange(' 28000 , 27015 - 27020, 27018-27025, 27026'), [
        {from: 27015, to: 27026},
        {from: 28000, to: 28000},
    ])
    assert.deepEqual(parsePortRange('1-65535'), [{from: 1, to: 65535}])
})

test('blank text is an empty pool', () => {
    assert.deepEqual(parsePortRange(''), [])
    assert.deepEqual(parsePortRange('   '), [])
    assert.deepEqual(parsePortRange(undefined), [])
})

for (const text of ['0', '27015-65536', '27020-27015', '27015,,27020', '27015,', '27015-', '27015--27020', '+27015', 'game', '27015;27020']) {
    test(`rejects ${JSON.stringify(text)} like the API does`, () => {
        assert.equal(parsePortRange(text), null)
    })
}

test('inPortRange checks every span', () => {
    const spans = parsePortRange('27015-27020, 28000')

    assert.equal(inPortRange(spans, 27020), true)
    assert.equal(inPortRange(spans, 28000), true)
    assert.equal(inPortRange(spans, 27021), false)
    assert.equal(inPortRange([], 27015), false)
})

test('walks the pool from the start port and wraps to its beginning', () => {
    const spans = parsePortRange('27015-27020')
    const busy = new Set([27018, 27019, 27020])

    assert.equal(findFreePort(spans, 27017, (port) => !busy.has(port)), 27017)
    assert.equal(findFreePort(spans, 27018, (port) => !busy.has(port)), 27015)
})

test('starts at the pool when the start port lies outside it', () => {
    const spans = parsePortRange('30000-30010')

    assert.equal(findFreePort(spans, 27015, (port) => port !== 30000), 30001)
})

test('counts up from the start port without a pool', () => {
    assert.equal(findFreePort([], 27015, (port) => port > 27016), 27017)
})

test('returns null when nothing fits', () => {
    assert.equal(findFreePort(parsePortRange('27015-27016'), 27015, () => false), null)
})
