import type { ServerData } from '@gameap/plugin-sdk'

export const minecraftServer: ServerData = {
    id: 1,
    uuid: 'mc-server-uuid-1234-5678-90ab',
    name: 'Minecraft Survival',
    game_id: 'minecraft',
    game_mod_id: 1,
    ip: '192.168.1.100',
    port: 25565,
    query_port: 25565,
    rcon_port: 25575,
    enabled: true,
    installed: true,
    blocked: false,
    start_command: 'java -Xmx2G -jar server.jar nogui',
    dir: '/home/gameap/servers/minecraft',
    process_active: true,
    last_process_check: new Date().toISOString(),
}

export const csServer: ServerData = {
    id: 2,
    uuid: 'cs-server-uuid-5678-1234-cdef',
    name: 'CS2 Competitive',
    game_id: 'cs2',
    game_mod_id: 2,
    ip: '192.168.1.101',
    port: 27015,
    query_port: 27015,
    rcon_port: 27015,
    enabled: true,
    installed: true,
    blocked: false,
    start_command: './cs2 -dedicated +map de_dust2',
    dir: '/home/gameap/servers/cs2',
    process_active: false,
    last_process_check: new Date().toISOString(),
}

export const serverMocks: Record<string, ServerData> = {
    minecraft: minecraftServer,
    cs: csServer,
}

export const serverAbilities: Record<string, string[]> = {
    minecraft: ['console-view', 'files-view', 'settings-view', 'start-server', 'stop-server', 'restart-server'],
    cs: ['console-view', 'files-view', 'settings-view', 'rcon', 'start-server', 'stop-server', 'restart-server'],
}
