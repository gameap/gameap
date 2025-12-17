import type { UserData } from '@gameap/plugin-sdk'

export const adminUser: UserData = {
    id: 1,
    login: 'admin',
    name: 'Administrator',
    roles: ['admin', 'user'],
    isAdmin: true,
    isAuthenticated: true,
}

export const regularUser: UserData = {
    id: 2,
    login: 'player1',
    name: 'Regular Player',
    roles: ['user'],
    isAdmin: false,
    isAuthenticated: true,
}

export const guestUser: UserData = {
    id: 0,
    login: '',
    name: 'Guest',
    roles: [],
    isAdmin: false,
    isAuthenticated: false,
}

export const userMocks: Record<string, UserData> = {
    admin: adminUser,
    user: regularUser,
    guest: guestUser,
}
