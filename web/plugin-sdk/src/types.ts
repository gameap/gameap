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
    /** Custom file editors to register in file manager */
    fileEditors?: PluginFileEditor[];
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
 * Permission check for hasServerPermissions.
 * Requires user to have all specified permissions for the server.
 */
export interface HasServerPermissionsCheck {
    type: 'hasServerPermissions';
    permissions: string[];
}

/**
 * Union type for all permission checks.
 * Each slot recipient decides which check types it supports.
 */
export type PermissionCheck = HasServerPermissionsCheck;

/**
 * Game match condition for slot components.
 * Supported by the server-tabs slot: the tab is shown only when the
 * current server's game matches at least one of the listed values.
 */
export interface GameCheck {
    /** Game engines to match (case-insensitive), e.g. ['GoldSource'] */
    engines?: string[];
    /** Game codes to match, e.g. ['cstrike', 'valve'] */
    codes?: string[];
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
    /** Permission check - each slot recipient checks types it understands */
    checkPermission?: PermissionCheck;
    /** Game match condition - each slot recipient checks it if supported */
    checkGame?: GameCheck;
}

/**
 * Available slot names in GameAP.
 */
export type SlotName =
    | 'server-tabs'
    | 'server-control-buttons'
    | 'server-control-blocks'
    | 'servers-list-actions'
    | 'dashboard-widgets'
    | 'sidebar-sections'
    | 'navbar-items'
    | 'global-banners'
    | 'admin-pages'
    | 'home-buttons'
    | 'profile-info-rows'
    | 'profile-blocks'
    | 'admin-user-info'
    | 'admin-user-info-above'
    | 'admin-user-info-rows'
    | 'admin-user-edit-blocks'
    | 'admin-node-edit-blocks'
    | 'admin-server-edit-blocks'
    | 'admin-game-edit-blocks'
    | 'admin-mod-edit-blocks';

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

/**
 * Props passed to server control buttons and blocks on the server page.
 *
 * Both slots evaluate `checkPermission` / `checkGame`, so a component that
 * declares them is only rendered when the viewer actually has the abilities.
 */
export interface ServerControlProps {
    serverId: number;
    server: ServerData;
    abilities: Record<string, boolean>;
    pluginId: string;
}

/**
 * Props passed to components in the `servers-list-actions` slot, rendered in the
 * commands column of the server list. `checkPermission` / `checkGame` apply.
 */
export interface ServersListActionProps {
    serverId: number;
    server: ServerData;
    pluginId: string;
}

/**
 * Props passed to navbar items and global banners.
 */
export interface ChromeSlotProps {
    routeName: string | null;
    isAdmin: boolean;
    pluginId: string;
}

/**
 * Props passed to sidebar sections. `minimized` tells whether the sidebar is
 * collapsed to icons.
 */
export interface SidebarSectionProps {
    minimized: boolean;
    isAdmin: boolean;
    pluginId: string;
}

/**
 * Props passed to the profile slots (`profile-info-rows`, `profile-blocks`).
 *
 * A `profile-info-rows` component MUST render a `<tr>` as its root element -
 * it is placed directly inside the profile table body.
 */
export interface ProfileSlotProps {
    userId: number;
    user: UserData;
    pluginId: string;
}

/**
 * Props passed to the user slots in the admin user modal
 * (`admin-user-info-above`, `admin-user-info-rows`, `admin-user-info`).
 *
 * An `admin-user-info-rows` component MUST render a `<tr>` as its root element.
 */
export interface AdminUserInfoProps {
    userId: number;
    user: UserData;
    pluginId: string;
}

/**
 * Editable user fields, as a read-only snapshot. Password fields are never
 * included.
 */
export interface UserEditFormData {
    readonly login: string;
    readonly name: string;
    readonly email: string;
    readonly roles: readonly string[];
    readonly servers: readonly { readonly id: number; readonly name: string }[];
}

/**
 * Props passed to components in the `admin-user-edit-blocks` slot. `form`
 * reflects the unsaved state of the edit form.
 */
export interface AdminUserEditBlockProps extends AdminUserInfoProps {
    form: UserEditFormData;
}

/**
 * Props passed to components in the `admin-node-edit-blocks` slot. Daemon
 * credentials, certificates and control scripts are never included in `form`.
 */
export interface AdminNodeEditBlockProps {
    nodeId: number;
    form: Record<string, unknown>;
    pluginId: string;
}

/**
 * Identity and saved state of the edited server. Credentials, the RCON password
 * included, are never part of it.
 */
export interface AdminServerSavedData {
    id: number;
    uuid: string;
    uuid_short: string;
    name: string;
    enabled: boolean;
    installed: boolean;
    blocked: boolean;
    online: boolean;
    ds_id: number;
    game_id: string;
    game_mod_id: number;
}

/**
 * Props passed to components in the `admin-server-edit-blocks` slot. The RCON
 * password is never included, neither in `form` nor in `server`.
 */
export interface AdminServerEditBlockProps {
    serverId: number;
    server: AdminServerSavedData | null;
    form: Record<string, unknown>;
    pluginId: string;
}

/**
 * Props passed to components in the `admin-game-edit-blocks` slot.
 */
export interface AdminGameEditBlockProps {
    gameCode: string;
    form: Record<string, unknown>;
    pluginId: string;
}

/**
 * Props passed to components in the `admin-mod-edit-blocks` slot.
 */
export interface AdminModEditBlockProps {
    modId: number;
    form: Record<string, unknown>;
    pluginId: string;
}

/**
 * Content type that the editor can handle.
 *
 * `none` means the modal loads nothing before mounting the editor: the editor
 * fetches whatever it needs itself, and the file manager's edit size cap does
 * not apply to it. It is what an editor that only inspects a file — its size,
 * its history — needs in order to work on a file too large to hand over.
 */
export type EditorContentType = 'text' | 'binary' | 'none';

/**
 * A block of the file manager's context menu, as its dividers separate them.
 *
 * - `top` — a block of its own above the built-in ones. Nothing of the file
 *   manager's is in it; it is where plugin items went before there was a
 *   choice, so it is what an editor gets when it names no block, and what it
 *   gets when it names one the panel has never heard of.
 * - `open` — Open, Play, View, Edit, Select, Download, Zip, Unzip.
 * - `modify` — Copy, Cut, Rename, Chmod, Paste.
 * - `danger` — Delete.
 * - `info` — Checksums, Properties. The one block whose plugin items go above
 *   its own, so that Properties stays the last line of the menu.
 */
export type EditorMenuGroup = 'top' | 'open' | 'modify' | 'danger' | 'info';

/**
 * Matching rules for when a file editor should be available.
 * Multiple rules can be specified - all provided rules must match (AND logic).
 */
export interface EditorMatchRules {
    /** Match all files (lowest specificity, score=1) */
    allFiles?: boolean;
    /** Exact file name match (e.g., "server.properties") */
    fileName?: string;
    /** Partial path match - file path must contain this string (e.g., "amxmodx/configs/") */
    pathContains?: string;
    /** Exact full path match (e.g., "/cstrike/server.cfg") */
    fullPath?: string;
    /** Array of file extensions to match (e.g., ["ini", "cfg", "json"]) */
    extensions?: string[];
    /** Regex pattern for file name matching */
    fileNameRegexp?: string;
    /** Game code filter - only match for servers with this game_id */
    gameCode?: string;
    /** Game name filter - only match for servers with this game name */
    gameName?: string;
}

/**
 * Props passed to file editor components.
 */
export interface FileEditorProps {
    /**
     * File content (string for text, ArrayBuffer for binary).
     * Absent when the editor declares `contentType: 'none'`.
     */
    content?: string | ArrayBuffer;
    /** Full file path */
    filePath: string;
    /** File name with extension */
    fileName: string;
    /** File extension (without dot) */
    extension: string;
    /** Current server's game code (if available) */
    gameCode?: string;
    /** Current server's game name (if available) */
    gameName?: string;
    /** Size in bytes, as the directory listing reported it */
    fileSize?: number;
    /** Modification time in unix seconds, as the directory listing reported it */
    fileMtime?: number;
    /** File manager disk the file lives on */
    disk?: string;
    /** ID of the plugin that registered this editor */
    pluginId: string;
}

/**
 * Plugin file editor registration definition.
 */
export interface PluginFileEditor {
    /** Unique identifier for this editor within the plugin */
    id: string;
    /** Display name shown in context menu (e.g., "Server Config Editor") */
    name: string;
    /** Vue component that renders the editor */
    component: Component;
    /** Matching rules that determine when this editor is available */
    match: EditorMatchRules;
    /** Content type: 'text' (default), 'binary' or 'none' */
    contentType?: EditorContentType;
    /** If true, editor is read-only (no save button) */
    readOnly?: boolean;
    /** Icon name from the @gameap/ui icon registry (e.g. 'file-archive') */
    icon?: string;
    /**
     * Keep the editor out of the double-click default: it is offered in the
     * context menu only, and the file manager's own handling of a double click
     * (image, video, pdf, text) stays as it was. An editor that matches every
     * file has to set this, or it takes every preview over.
     */
    contextMenuOnly?: boolean;
    /**
     * Caption of the context menu item, instead of the "Edit with <name>"
     * wording. Supports the `@:key` form, resolved through the plugin's own
     * translations.
     */
    menuLabel?: string;
    /**
     * Which block of the context menu the item joins. It lands after that
     * block's own items, except in `info`, where it goes above them so that
     * Properties stays the last line of the menu. The default is `top`, a
     * block of its own above the rest, which is where every plugin item sat
     * before this field existed — so leaving it out keeps an editor exactly
     * where it was, and a panel older than the field puts it there whatever
     * is named.
     */
    menuGroup?: EditorMenuGroup;
    /**
     * Permission check - the file manager hides the item when it fails, so a
     * plugin does not offer what its own API would answer 403 to.
     */
    checkPermission?: PermissionCheck;
    /**
     * Width of the modal as a CSS length, e.g. `'min(1400px, 95vw)'`. The
     * default is 1000px, which is narrow for anything shown side by side. No
     * max-width is applied, so a viewport-relative value is the safe form.
     */
    width?: string;
    /**
     * Take the modal's own footer away. An editor that draws its actions
     * inside gets the height back, and closing stays on the header's cross and
     * on Escape.
     */
    hideFooter?: boolean;
    /**
     * Leave the modal open after a successful save instead of closing it. The
     * editor's exposed `onSaved()` is called either way, which is how an
     * editor that stays open reports the write it cannot see itself.
     */
    keepOpenOnSave?: boolean;
}
