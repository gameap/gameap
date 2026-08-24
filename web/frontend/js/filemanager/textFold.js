/**
 * Text folding for the file search: matching ignores case and diacritics, so
 * "е" finds "ё", "c" finds "ć", "z" finds "ž", "s" finds "š" and so on.
 *
 * Most letters fold through NFD decomposition — the combining marks are simply
 * dropped. The map below covers the ones that have no decomposition.
 */
const SPECIAL = {
    ß: 'ss',
    ø: 'o',
    đ: 'd',
    ð: 'd',
    ł: 'l',
    æ: 'ae',
    œ: 'oe',
    þ: 'th',
    ħ: 'h',
    ı: 'i',
}

const COMBINING = /[\u0300-\u036f]/g

function foldChar(char) {
    const lower = char.toLowerCase()
    if (SPECIAL[lower]) return SPECIAL[lower]

    const stripped = lower.normalize('NFD').replace(COMBINING, '')

    return stripped === '' ? lower : stripped
}

export function foldText(text) {
    let result = ''
    for (const char of text) {
        result += foldChar(char)
    }

    return result
}

/**
 * Folded text plus, for every folded character, the index of the source
 * character it came from — a fold may drop or add characters, so positions
 * cannot be reused directly.
 */
function foldWithMap(text) {
    let folded = ''
    const origin = []
    let index = 0

    for (const char of text) {
        const piece = foldChar(char)
        for (let i = 0; i < piece.length; i += 1) {
            origin.push(index)
        }
        folded += piece
        index += char.length
    }

    return { folded, origin }
}

/**
 * Ranges of `text` matching `query`, in source-string coordinates.
 *
 * @param {string} text
 * @param {string} query
 * @returns {Array<{start: number, end: number}>}
 */
export function findMatchRanges(text, query) {
    const needle = foldText(query)
    if (needle === '') return []

    const { folded, origin } = foldWithMap(text)
    const ranges = []
    let position = 0
    let found = folded.indexOf(needle, position)

    while (found !== -1) {
        const after = found + needle.length
        ranges.push({
            start: origin[found],
            end: after < origin.length ? origin[after] : text.length,
        })
        position = after
        found = folded.indexOf(needle, position)
    }

    return ranges
}
