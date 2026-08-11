// Server-side archive constants shared by the context menu, modals and the
// operations store. Format values mirror the backend API names.

export const CREATE_FORMATS = [
    { value: 'zip', suffix: '.zip' },
    { value: 'tar', suffix: '.tar' },
    { value: 'tar_gz', suffix: '.tar.gz' },
    { value: 'tar_bz2', suffix: '.tar.bz2' },
    { value: 'tar_xz', suffix: '.tar.xz' },
    { value: 'tar_zstd', suffix: '.tar.zst' },
]

// Longest-first so "backup.tar.gz" matches ".tar.gz" before ".gz".
export const EXTRACT_SUFFIXES = [
    '.tar.bz2',
    '.tar.gz',
    '.tar.xz',
    '.tar.zst',
    '.tbz2',
    '.tgz',
    '.txz',
    '.zip',
    '.tar',
    '.bz2',
    '.gz',
    '.xz',
    '.zst',
    '.7z',
    '.rar',
]

const CREATE_SUFFIXES = [
    { suffix: '.tar.gz', value: 'tar_gz' },
    { suffix: '.tar.bz2', value: 'tar_bz2' },
    { suffix: '.tar.xz', value: 'tar_xz' },
    { suffix: '.tar.zst', value: 'tar_zstd' },
    { suffix: '.tgz', value: 'tar_gz' },
    { suffix: '.txz', value: 'tar_xz' },
    { suffix: '.zip', value: 'zip' },
    { suffix: '.tar', value: 'tar' },
]

export function matchArchiveSuffix(basename) {
    if (!basename) return null
    const lowered = basename.toLowerCase()

    return EXTRACT_SUFFIXES.find((s) => lowered.endsWith(s) && lowered.length > s.length) || null
}

export function isExtractable(basename) {
    return matchArchiveSuffix(basename) !== null
}

export function stripArchiveSuffix(basename) {
    const suffix = matchArchiveSuffix(basename)
    if (!suffix) return basename

    return basename.slice(0, basename.length - suffix.length)
}

// Resolves the create-format from an archive name; null when the extension
// is not one the panel can create.
export function deriveCreateFormat(name) {
    if (!name) return null
    const lowered = name.toLowerCase()
    const match = CREATE_SUFFIXES.find((s) => lowered.endsWith(s.suffix) && lowered.length > s.suffix.length)

    return match ? match.value : null
}

export function replaceCreateSuffix(name, format) {
    const target = CREATE_FORMATS.find((f) => f.value === format)
    if (!target) return name

    const current = CREATE_SUFFIXES.find((s) => name.toLowerCase().endsWith(s.suffix))
    const base = current ? name.slice(0, name.length - current.suffix.length) : name

    return `${base || 'archive'}${target.suffix}`
}
