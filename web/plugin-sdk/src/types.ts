import type { Component, ComputedRef } from 'vue';

/**
 * Main plugin definition interface.
 * This is what plugin developers export from their plugin's entry point.
 */
export interface PluginDefinition {
    /** Unique plugin identifier (e.g., 'my-awesome-plugin') */
    id: string;
    /** Human-readable plugin name */
    name: string;
    /** Semantic version string (e.g., '1.0.0') */
    version: string;
    /** GameAP plugin API version (currently '1.0') */
    apiVersion: '1.0';
    /** Optional plugin description */
    description?: string;
    /** Optional plugin author */
    author?: string;
    /** Plugin routes to register */
    routes?: PluginRoute[];
    /** Menu items to add to the sidebar */
    menuItems?: PluginMenuItem[];
    /** Components to inject into named slots */
    slots?: Record<string, PluginSlotComponent[]>;
    /** Home page buttons */
    homeButtons?: PluginHomeButton[];
    /** Plugin translations keyed by language code (e.g., { en: { key: 'value' }, ru: { key: 'значение' } }) */
    translations?: Record<string, Record<string, string>>;
    /** Plugin initialization hook */
    onInit?: () => void | Promise<void>;
    /** Plugin cleanup hook */
    onDestroy?: () => void | Promise<void>;
}

/**
 * Plugin home button definition.
 * Buttons appear on the home page next to Servers and Nodes.
 */
export interface PluginHomeButton {
    /** Button display name (required) */
    name: string;
    /** Font Awesome icon class (optional, defaults to 'fas fa-puzzle-piece') */
    icon?: string;
    /** Custom Vue component to render instead of default button (optional) */
    component?: Component;
    /** Route to navigate to (optional, defaults to plugin index route) */
    route?: { name: string } | { path: string };
    /** Sort order (lower numbers appear first) */
    order?: number;
}

/**
 * Plugin route definition.
 */
export interface PluginRoute {
    /** Route path relative to /plugins/{pluginId}/ */
    path: string;
    /** Route name (will be prefixed with plugin.{pluginId}.) */
    name: string;
    /** Vue component to render */
    component: Component;
    /** Optional route metadata */
    meta?: {
        title?: string;
        requiresAuth?: boolean;
        requiresAdmin?: boolean;
        [key: string]: unknown;
    };
    /** Nested child routes */
    children?: PluginRoute[];
}

/**
 * Plugin menu item definition.
 */
export interface PluginMenuItem {
    /** Menu section to add the item to */
    section: 'servers' | 'admin' | 'custom';
    /** Font Awesome icon class (e.g., 'fas fa-puzzle-piece') */
    icon?: string;
    /** Menu item display text */
    text: string;
    /** Route to navigate to when clicked */
    route: { name: string } | { path: string };
    /** Sort order (lower numbers appear first) */
    order?: number;
    /** Only show if user is admin */
    adminOnly?: boolean;
}

/**
 * Plugin slot component definition.
 */
export interface PluginSlotComponent {
    /** Vue component to render in the slot */
    component: Component;
    /** Sort order within the slot (lower numbers appear first) */
    order?: number;
    /** Display label for the component */
    label?: string;
    /** Font Awesome icon class */
    icon?: string;
    /** Unique name for the component within this slot */
    name?: string;
    /** Default props to pass to the component */
    props?: Record<string, unknown>;
}

/**
 * Available slot names in GameAP.
 */
export type SlotName =
    | 'server-tabs'
    | 'dashboard-widgets'
    | 'sidebar-sections'
    | 'admin-pages';

/**
 * Server data available to plugins.
 */
export interface ServerData {
    id: number;
    uuid: string;
    name: string;
    game_id: string;
    game_mod_id: number;
    ip: string;
    port: number;
    query_port: number;
    rcon_port: number;
    enabled: boolean;
    installed: boolean;
    blocked: boolean;
    start_command: string;
    dir: string;
    process_active: boolean;
    last_process_check: string;
}

/**
 * User data available to plugins.
 */
export interface UserData {
    id: number;
    login: string;
    name: string;
    roles: string[];
    isAdmin: boolean;
    isAuthenticated: boolean;
}

/**
 * Route info available in plugin context.
 */
export interface PluginRouteInfo {
    name: string | null;
    path: string;
    params: Record<string, string>;
    query: Record<string, string>;
    pluginId: string | null;
}

/**
 * Plugin context provided to plugin components.
 */
export interface PluginContext {
    /** Current route information */
    route: ComputedRef<PluginRouteInfo>;
    /** Current server data (when on server pages) */
    server: ComputedRef<{
        id: number | null;
        data: ServerData | null;
        abilities: string[];
    }>;
    /** Current user information */
    user: ComputedRef<UserData>;
    /** Direct access to Pinia stores */
    stores: {
        auth: unknown;
        server: unknown;
        plugins: unknown;
    };
}

/**
 * Props passed to server tab components.
 */
export interface ServerTabProps {
    serverId: number;
    server: ServerData;
    pluginId: string;
}

/**
 * Props passed to dashboard widget components.
 */
export interface DashboardWidgetProps {
    isAdmin: boolean;
    pluginId: string;
}
