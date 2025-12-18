import { http, HttpResponse, delay } from 'msw'
import { mockFiles, mockDirectories, mockSubdirectoryFiles, mockSubdirectories, getFileByPath, type MockFile, type MockDirectory } from './files'
import { serverMocks, serverAbilities, mockServersList, mockServersDetails, type ServerListItem } from './servers'
import { userMocks } from './users'
import type { ServerData, UserData } from '@gameap/plugin-sdk'

// Import actual translation files (copied from backend)
import translationsEn from './translations-en.json'
import translationsRu from './translations-ru.json'

const translations: Record<string, object> = {
    en: translationsEn,
    ru: translationsRu,
}

// Initialize debug state from localStorage (for persistence across reloads)
function getInitialUserType(): 'admin' | 'user' | 'guest' {
    if (typeof localStorage !== 'undefined') {
        const stored = localStorage.getItem('gameap_debug_user_type')
        if (stored === 'admin' || stored === 'user' || stored === 'guest') {
            return stored
        }
    }
    return 'admin'
}

// Current debug state (will be controlled by debug panel)
export const debugState = {
    userType: getInitialUserType(),
    serverId: 1,
    locale: 'en' as 'en' | 'ru',
    networkDelay: 100, // ms
}

// Helper to get current user
function getCurrentUser(): UserData {
    return userMocks[debugState.userType]
}

// Get server by ID from the list (for API responses)
function getServerListItemById(id: number): ServerListItem | undefined {
    return mockServersList.find(s => s.id === id)
}

// Get server data for plugin context
function getServerById(id: number): ServerData | null {
    if (id === 1) return serverMocks.minecraft
    if (id === 2) return serverMocks.cs
    return null
}

// Helper to get files and directories for a path
function getFilesForPath(path: string): { files: MockFile[], directories: MockDirectory[] } {
    // Normalize path: empty string or '/' means root
    const normalizedPath = path === '/' ? '' : path.replace(/^\//, '').replace(/\/$/, '')

    if (normalizedPath === '') {
        // Root directory
        return {
            files: mockFiles.map(f => ({
                path: f.path,
                timestamp: f.timestamp,
                type: f.type,
                visibility: f.visibility,
                size: f.size,
                dirname: f.dirname,
                basename: f.basename,
                extension: f.extension,
                filename: f.filename,
            })),
            directories: mockDirectories,
        }
    }

    // Subdirectory
    const files = mockSubdirectoryFiles[normalizedPath] || []
    const directories = mockSubdirectories[normalizedPath] || []

    return {
        files: files.map(f => ({
            path: f.path,
            timestamp: f.timestamp,
            type: f.type,
            visibility: f.visibility,
            size: f.size,
            dirname: f.dirname,
            basename: f.basename,
            extension: f.extension,
            filename: f.filename,
        })),
        directories,
    }
}

// Mock games
const mockGames = [
    { code: 'minecraft', name: 'Minecraft', engine: 'source', engine_version: '1.0', steam_app_id: null },
    { code: 'cs2', name: 'Counter-Strike 2', engine: 'source2', engine_version: '1.0', steam_app_id: 730 },
]

// Mock game mods
const mockGameMods = [
    { id: 1, game_code: 'minecraft', name: 'Vanilla', default_start_cmd: 'java -jar server.jar' },
    { id: 2, game_code: 'cs2', name: 'Competitive', default_start_cmd: './cs2 -dedicated' },
]

// Mock nodes
const mockNodes = [
    {
        id: 1,
        name: 'Local Node',
        enabled: true,
        ip: ['192.168.1.100'],
        os: 'linux',
        location: 'Local',
        gdaemon_host: '192.168.1.100',
        gdaemon_port: 31717,
        gdaemon_api_key: '***',
        gdaemon_version: '3.1.0',
        work_path: '/home/gameap',
        steamcmd_path: '/home/gameap/steamcmd',
        servers_count: 2,
    },
]

// Mock users list
const mockUsersList = [
    { id: 1, login: 'admin', name: 'Administrator', roles: ['admin'], created_at: '2024-01-01' },
    { id: 2, login: 'player1', name: 'Regular Player', roles: ['user'], created_at: '2024-01-15' },
]

// Mock console output
let consoleOutput = [
    '[00:00:01] Server starting...',
    '[00:00:02] Loading world...',
    '[00:00:05] Done! Server ready.',
    '[00:00:10] Player1 joined the game',
]

// Plugin JS/CSS content (empty by default, will be injected when plugin is loaded)
let pluginJsContent = ''
let pluginCssContent = ''

export function setPluginContent(js: string, css: string) {
    pluginJsContent = js
    pluginCssContent = css
}

export const handlers = [
    // ==================== Auth & Profile ====================
    http.get('/api/profile', async () => {
        await delay(debugState.networkDelay)
        const user = getCurrentUser()
        if (!user.isAuthenticated) {
            return new HttpResponse(null, { status: 401 })
        }
        return HttpResponse.json({
            id: user.id,
            login: user.login,
            name: user.name,
            roles: user.roles,
        })
    }),

    http.put('/api/profile', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true })
    }),

    http.get('/api/user/servers_abilities', async () => {
        await delay(debugState.networkDelay)
        // Return abilities for all servers the user can access
        const abilities: Record<number, Record<string, boolean>> = {}
        mockServersList.forEach(s => {
            abilities[s.id] = serverAbilities[s.id] || {
                'game-server-common': true,
                'game-server-start': true,
                'game-server-stop': true,
                'game-server-restart': true,
            }
        })
        return HttpResponse.json(abilities)
    }),

    http.post('/api/password/reset', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true })
    }),

    // ==================== Servers ====================
    http.get('/api/servers', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json(mockServersList)
    }),

    http.get('/api/servers/summary', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({
            total: mockServersList.length,
            active: mockServersList.filter(s => s.process_active).length,
            inactive: mockServersList.filter(s => !s.process_active).length,
        })
    }),

    http.get('/api/servers/search', async ({ request }) => {
        await delay(debugState.networkDelay)
        const url = new URL(request.url)
        const q = url.searchParams.get('q') || ''
        const filtered = mockServersList.filter(s =>
            s.name.toLowerCase().includes(q.toLowerCase())
        )
        return HttpResponse.json(filtered)
    }),

    http.get('/api/servers/:id', async ({ params }) => {
        await delay(debugState.networkDelay)
        const serverId = Number(params.id)
        const serverDetails = mockServersDetails[serverId]
        if (!serverDetails) {
            return new HttpResponse(null, { status: 404 })
        }

        // Check if user is admin - return full details
        const user = getCurrentUser()
        if (user.roles.includes('admin')) {
            return HttpResponse.json(serverDetails)
        }

        // For regular users, return limited info
        const {
            id, enabled, installed, blocked, name, game_id, game_mod_id,
            expires, server_ip, server_port, query_port, rcon_port,
            game, last_process_check, online, process_active
        } = serverDetails

        return HttpResponse.json({
            id, enabled, installed, blocked, name, game_id, game_mod_id,
            expires, server_ip, server_port, query_port, rcon_port,
            game: {
                code: game.code,
                name: game.name,
                engine: game.engine,
                engine_version: game.engine_version,
            },
            last_process_check, online, process_active,
        })
    }),

    http.get('/api/servers/:id/abilities', async ({ params }) => {
        await delay(debugState.networkDelay)
        const serverId = Number(params.id)
        const abilities = serverAbilities[serverId]
        if (!abilities) {
            // Return default abilities for unknown servers
            return HttpResponse.json({
                'game-server-common': true,
                'game-server-start': true,
                'game-server-stop': true,
                'game-server-restart': true,
            })
        }
        return HttpResponse.json(abilities)
    }),

    http.get('/api/servers/:id/settings', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({
            vars: {
                maxplayers: '20',
                hostname: 'Test Server',
            },
        })
    }),

    http.put('/api/servers/:id', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true })
    }),

    http.put('/api/servers/:id/settings', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true })
    }),

    http.post('/api/servers', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true, id: 3 })
    }),

    http.post('/api/servers/reinstall', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true, task_id: 100 })
    }),

    // Server control
    http.post('/api/servers/:id/:action', async ({ params }) => {
        await delay(debugState.networkDelay)
        const action = params.action as string
        if (['start', 'stop', 'restart', 'update', 'reinstall'].includes(action)) {
            return HttpResponse.json({
                success: true,
                gdaemon_task_id: Math.floor(Math.random() * 1000)
            })
        }
        return new HttpResponse(null, { status: 400 })
    }),

    http.get('/api/servers/:id/status', async ({ params }) => {
        await delay(debugState.networkDelay)
        const server = getServerById(Number(params.id))
        return HttpResponse.json({
            status: server?.process_active ? 'active' : 'inactive',
            process_active: server?.process_active ?? false,
        })
    }),

    http.get('/api/servers/:id/query', async ({ params }) => {
        await delay(debugState.networkDelay)
        const server = getServerById(Number(params.id))
        if (!server?.process_active) {
            return HttpResponse.json({ online: false })
        }
        return HttpResponse.json({
            online: true,
            players: 5,
            max_players: 20,
            map: 'de_dust2',
            hostname: server.name,
        })
    }),

    // Server console
    http.get('/api/servers/:id/console', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({
            console: consoleOutput.join('\n'),
        })
    }),

    http.post('/api/servers/:id/console', async ({ request }) => {
        await delay(debugState.networkDelay)
        const body = await request.json() as { command: string }
        consoleOutput.push(`> ${body.command}`)
        consoleOutput.push(`[Server] Command executed: ${body.command}`)
        return HttpResponse.json({ success: true })
    }),

    // Server tasks
    http.get('/api/servers/:id/tasks', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json([
            { id: 1, command: 'restart', repeat: 'daily', repeat_period: 86400, execute_date: '2024-12-18 00:00:00' },
        ])
    }),

    http.post('/api/servers/:id/tasks', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true, id: 2 })
    }),

    http.put('/api/servers/:id/tasks/:taskId', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true })
    }),

    http.delete('/api/servers/:id/tasks/:taskId', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true })
    }),

    // RCON
    http.get('/api/servers/:id/rcon/features', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({
            players: true,
            fast_rcon: true,
            console: true,
        })
    }),

    http.get('/api/servers/:id/rcon/fast_rcon', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json([
            { command: 'say Hello', label: 'Say Hello' },
            { command: 'changelevel de_dust2', label: 'Change to Dust2' },
        ])
    }),

    http.post('/api/servers/:id/rcon', async ({ request }) => {
        await delay(debugState.networkDelay)
        const body = await request.json() as { command: string }
        return HttpResponse.json({
            output: `Executed: ${body.command}\nResult: OK`,
        })
    }),

    http.get('/api/servers/:id/rcon/players', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json([
            { id: 1, name: 'Player1', score: 10, ping: 45 },
            { id: 2, name: 'Player2', score: 8, ping: 60 },
        ])
    }),

    http.post('/api/servers/:id/rcon/players/kick', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true })
    }),

    http.post('/api/servers/:id/rcon/players/ban', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true })
    }),

    http.post('/api/servers/:id/rcon/players/message', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true })
    }),

    // ==================== Games ====================
    http.get('/api/games', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json(mockGames)
    }),

    http.get('/api/games/:code', async ({ params }) => {
        await delay(debugState.networkDelay)
        const game = mockGames.find(g => g.code === params.code)
        if (!game) {
            return new HttpResponse(null, { status: 404 })
        }
        return HttpResponse.json(game)
    }),

    http.get('/api/games/:code/mods', async ({ params }) => {
        await delay(debugState.networkDelay)
        const mods = mockGameMods.filter(m => m.game_code === params.code)
        return HttpResponse.json(mods)
    }),

    http.put('/api/games/:code', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true })
    }),

    http.post('/api/games', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true })
    }),

    http.delete('/api/games/:code', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true })
    }),

    // ==================== Game Mods ====================
    http.get('/api/game_mods', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json(mockGameMods)
    }),

    http.get('/api/game_mods/:id', async ({ params }) => {
        await delay(debugState.networkDelay)
        const mod = mockGameMods.find(m => m.id === Number(params.id))
        if (!mod) {
            return new HttpResponse(null, { status: 404 })
        }
        return HttpResponse.json(mod)
    }),

    http.get('/api/game_mods/get_list_for_game/:code', async ({ params }) => {
        await delay(debugState.networkDelay)
        const mods = mockGameMods.filter(m => m.game_code === params.code)
        return HttpResponse.json(mods)
    }),

    http.post('/api/game_mods', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true, id: 3 })
    }),

    http.put('/api/game_mods/:id', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true })
    }),

    http.delete('/api/game_mods/:id', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true })
    }),

    // ==================== Nodes (Dedicated Servers) ====================
    http.get('/api/dedicated_servers', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json(mockNodes)
    }),

    http.get('/api/dedicated_servers/summary', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({
            total: mockNodes.length,
            online: mockNodes.length,
        })
    }),

    http.get('/api/dedicated_servers/setup', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({
            script: '#!/bin/bash\necho "Install GDaemon"',
        })
    }),

    http.get('/api/dedicated_servers/:id', async ({ params }) => {
        await delay(debugState.networkDelay)
        const node = mockNodes.find(n => n.id === Number(params.id))
        if (!node) {
            return new HttpResponse(null, { status: 404 })
        }
        return HttpResponse.json(node)
    }),

    http.get('/api/dedicated_servers/:id/daemon', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({
            version: '3.1.0',
            uptime: 86400,
            status: 'online',
        })
    }),

    http.get('/api/dedicated_servers/:id/ip_list', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json(['192.168.1.100', '192.168.1.101'])
    }),

    http.get('/api/dedicated_servers/:id/busy_ports', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json([25565, 27015])
    }),

    http.post('/api/dedicated_servers', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true, id: 2 })
    }),

    http.put('/api/dedicated_servers/:id', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true })
    }),

    http.delete('/api/dedicated_servers/:id', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true })
    }),

    // ==================== Users ====================
    http.get('/api/users/', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json(mockUsersList)
    }),

    http.get('/api/users/:id', async ({ params }) => {
        await delay(debugState.networkDelay)
        const user = mockUsersList.find(u => u.id === Number(params.id))
        if (!user) {
            return new HttpResponse(null, { status: 404 })
        }
        return HttpResponse.json(user)
    }),

    http.get('/api/users/:id/servers', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json(mockServersList.map((s: ServerListItem) => ({ id: s.id, name: s.name })))
    }),

    http.get('/api/users/:id/servers/:serverId/permissions', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({
            'console-view': true,
            'files-view': true,
            'settings-view': true,
            'start-server': true,
            'stop-server': true,
            'restart-server': true,
        })
    }),

    http.post('/api/users', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true, id: 3 })
    }),

    http.put('/api/users/:id', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true })
    }),

    http.put('/api/users/:id/servers/:serverId/permissions', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true })
    }),

    http.delete('/api/users/:id', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true })
    }),

    // ==================== Tokens ====================
    http.get('/api/tokens', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json([
            { id: 1, name: 'API Token 1', abilities: ['*'], last_used_at: '2024-12-15' },
        ])
    }),

    http.get('/api/tokens/abilities', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json(['*', 'read', 'write', 'admin'])
    }),

    http.post('/api/tokens', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({
            token: 'mock-token-abc123xyz',
            id: 2,
        })
    }),

    http.delete('/api/tokens/:id', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true })
    }),

    // ==================== Client Certificates ====================
    http.get('/api/client_certificates', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json([
            { id: 1, fingerprint: 'AB:CD:EF:12:34', expires_at: '2025-12-01' },
        ])
    }),

    http.post('/api/client_certificates', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true, id: 2 })
    }),

    http.delete('/api/client_certificates/:id', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({ success: true })
    }),

    // ==================== GDaemon Tasks ====================
    http.get('/api/gdaemon_tasks', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({
            data: [
                { id: 1, task: 'server-start', status: 'success', server_id: 1, created_at: '2024-12-17 10:00:00' },
                { id: 2, task: 'server-stop', status: 'working', server_id: 1, created_at: '2024-12-17 11:00:00' },
            ],
            total: 2,
        })
    }),

    http.get('/api/gdaemon_tasks/:id', async ({ params }) => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({
            id: Number(params.id),
            task: 'server-start',
            status: 'success',
            output: 'Server started successfully',
        })
    }),

    http.get('/api/gdaemon_tasks/:id/output', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({
            output: 'Task output...\nCompleted successfully.',
        })
    }),

    // ==================== File Manager ====================
    // File manager initialize - new API path
    http.get('/api/file-manager/:serverId/initialize', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({
            result: {
                status: 'success',
                message: null,
            },
            config: {
                leftDisk: null,
                rightDisk: null,
                windowsConfig: 1,
                disks: {
                    server: {
                        driver: 'gameap',
                    },
                },
                lang: '',
            },
        })
    }),

    // File manager initialize - legacy API path
    http.get('/api/servers/:id/filemanager/initialize', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({
            result: {
                status: 'success',
                message: null,
            },
            config: {
                leftDisk: null,
                rightDisk: null,
                windowsConfig: 1,
                disks: {
                    server: {
                        driver: 'gameap',
                    },
                },
                lang: '',
            },
        })
    }),

    // File manager content - new API path
    http.get('/api/file-manager/:serverId/content', async ({ request }) => {
        await delay(debugState.networkDelay)
        const url = new URL(request.url)
        const path = url.searchParams.get('path') || ''

        const { files, directories } = getFilesForPath(path)

        return HttpResponse.json({
            result: {
                status: 'success',
                message: null,
            },
            directories,
            files,
        })
    }),

    // File manager content - legacy API path
    http.get('/api/servers/:id/filemanager/content', async ({ request }) => {
        await delay(debugState.networkDelay)
        const url = new URL(request.url)
        const path = url.searchParams.get('path') || ''

        const { files, directories } = getFilesForPath(path)

        return HttpResponse.json({
            result: {
                status: 'success',
                message: null,
            },
            directories,
            files,
        })
    }),

    http.get('/api/servers/:id/filemanager/tree', async ({ request }) => {
        await delay(debugState.networkDelay)
        const url = new URL(request.url)
        const path = url.searchParams.get('path') || ''

        const { directories } = getFilesForPath(path)

        return HttpResponse.json({
            result: { status: 'success', message: null },
            directories,
        })
    }),

    http.get('/api/servers/:id/filemanager/select-disk', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({
            result: { status: 'success' },
        })
    }),

    // File manager download - new API path
    http.get('/api/file-manager/:serverId/download', async ({ request }) => {
        await delay(debugState.networkDelay)
        const url = new URL(request.url)
        const path = url.searchParams.get('path') || ''

        const file = getFileByPath(path)
        if (!file) {
            return new HttpResponse(null, { status: 404 })
        }

        if (file._contentType === 'binary') {
            return new HttpResponse(file._content as ArrayBuffer, {
                headers: {
                    'Content-Type': 'application/octet-stream',
                },
            })
        }

        return new HttpResponse(file._content as string, {
            headers: {
                'Content-Type': 'text/plain',
            },
        })
    }),

    // File manager download - legacy API path
    http.get('/api/servers/:id/filemanager/download', async ({ request }) => {
        await delay(debugState.networkDelay)
        const url = new URL(request.url)
        const path = url.searchParams.get('path') || ''

        const file = getFileByPath(path)
        if (!file) {
            return new HttpResponse(null, { status: 404 })
        }

        if (file._contentType === 'binary') {
            return new HttpResponse(file._content as ArrayBuffer, {
                headers: {
                    'Content-Type': 'application/octet-stream',
                },
            })
        }

        return new HttpResponse(file._content as string, {
            headers: {
                'Content-Type': 'text/plain',
            },
        })
    }),

    http.post('/api/servers/:id/filemanager/update-file', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({
            result: { status: 'success' },
        })
    }),

    http.post('/api/servers/:id/filemanager/create-file', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({
            result: { status: 'success' },
        })
    }),

    http.post('/api/servers/:id/filemanager/create-directory', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({
            result: { status: 'success' },
        })
    }),

    http.post('/api/servers/:id/filemanager/delete', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({
            result: { status: 'success' },
        })
    }),

    http.post('/api/servers/:id/filemanager/rename', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({
            result: { status: 'success' },
        })
    }),

    http.post('/api/servers/:id/filemanager/paste', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({
            result: { status: 'success' },
        })
    }),

    http.post('/api/servers/:id/filemanager/upload', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({
            result: { status: 'success' },
        })
    }),

    http.post('/api/servers/:id/filemanager/zip', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({
            result: { status: 'success' },
        })
    }),

    http.post('/api/servers/:id/filemanager/unzip', async () => {
        await delay(debugState.networkDelay)
        return HttpResponse.json({
            result: { status: 'success' },
        })
    }),

    // ==================== Plugins ====================
    http.get('/plugins.js', async () => {
        await delay(debugState.networkDelay)
        if (!pluginJsContent) {
            return new HttpResponse('', { status: 404 })
        }
        return new HttpResponse(pluginJsContent, {
            headers: {
                'Content-Type': 'application/javascript',
            },
        })
    }),

    http.get('/plugins.css', async () => {
        await delay(debugState.networkDelay)
        if (!pluginCssContent) {
            return new HttpResponse('', { status: 404 })
        }
        return new HttpResponse(pluginCssContent, {
            headers: {
                'Content-Type': 'text/css',
            },
        })
    }),

    // ==================== Language ====================
    // Language/translations endpoint - uses actual translation files
    http.get('/lang/:locale.json', async ({ params }) => {
        await delay(debugState.networkDelay)
        const locale = (params.locale as string).replace('.json', '')
        const localeTranslations = translations[locale] || translations.en
        return HttpResponse.json(localeTranslations)
    }),

    // Also handle without .json extension
    http.get('/lang/:locale', async ({ params }) => {
        await delay(debugState.networkDelay)
        const locale = params.locale as string
        const localeTranslations = translations[locale] || translations.en
        return HttpResponse.json(localeTranslations)
    }),
]

export default handlers
