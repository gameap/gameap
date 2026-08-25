/**
 * Keyboard layout rescue for the file search: a query typed with the wrong
 * layout active ("ыукмук" instead of "server") is re-read as if it had been
 * typed on another layout.
 *
 * Every string below lists the characters produced by the same physical keys
 * in the same order — the 26 letter keys followed by the punctuation keys
 * [ ] ; ' , . / - = ` — so the position of a character is its physical key.
 * Digits are identical everywhere and stay untouched.
 */
const LAYOUTS = {
    en: "qwertyuiop[]asdfghjkl;'zxcvbnm,./-=`",
    ru: 'йцукенгшщзхъфывапролджэячсмитьбю.-=ё',
    de: 'qwertzuiopü+asdfghjklöäyxcvbnm,.-ß´^',
    es: "qwertyuiop`+asdfghjklñ´zxcvbnm,.-'¡º",
}

const CODES = Object.keys(LAYOUTS)

const chars = {}
const positions = {}

CODES.forEach((code) => {
    chars[code] = [...LAYOUTS[code]]
    positions[code] = new Map()
    chars[code].forEach((char, index) => {
        if (!positions[code].has(char)) {
            positions[code].set(char, index)
        }
    })
})

function convert(query, from, to) {
    const source = positions[from]
    const target = chars[to]
    let result = ''

    for (const char of query) {
        const index = source.get(char)
        result += index === undefined ? char : target[index]
    }

    return result
}

/**
 * Lowercased readings of the query, the query itself first.
 *
 * @param {string} query
 * @returns {string[]}
 */
export function queryVariants(query) {
    const raw = query.toLowerCase()
    const variants = [raw]
    if (raw === '') return variants

    CODES.forEach((from) => {
        CODES.forEach((to) => {
            if (from === to) return

            const converted = convert(raw, from, to)
            if (!variants.includes(converted)) {
                variants.push(converted)
            }
        })
    })

    return variants
}
