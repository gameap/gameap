// displayVersion strips the tag-style "v" prefix daemons and release feeds
// may carry ("v4.1.1" -> "4.1.1") so every version renders in one shape.
export function displayVersion(value) {
    if (!value || typeof value !== 'string') {
        return ''
    }

    return value.trim().replace(/^v(?=\d)/i, '')
}
