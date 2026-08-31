/**
 * Shared evaluation of the `checkPermission` / `checkGame` conditions that plugins
 * declare on their slot components. Slot hosts opt into the check explicitly:
 * a host that does not know the server context leaves the component visible.
 */

export function matchesPermission(checkPermission, abilities) {
    if (!checkPermission) {
        return true
    }

    if (checkPermission.type === 'hasServerPermissions') {
        const permissions = checkPermission.permissions ?? []

        return permissions.every(perm => (abilities ?? {})[perm] === true)
    }

    return true
}

export function matchesGame(checkGame, game) {
    if (!checkGame) {
        return true
    }

    if (!game) {
        return false
    }

    const engines = checkGame.engines ?? []
    const codes = checkGame.codes ?? []

    if (engines.length === 0 && codes.length === 0) {
        return true
    }

    const engineMatches = engines.some(
        engine => String(engine).toLowerCase() === String(game.engine ?? '').toLowerCase()
    )

    return engineMatches || codes.includes(game.code)
}

export function filterSlotComponents(components, { abilities, game } = {}) {
    return components.filter(
        item => matchesGame(item.checkGame, game) && matchesPermission(item.checkPermission, abilities)
    )
}
