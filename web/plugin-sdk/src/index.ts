/**
 * GameAP Plugin SDK
 *
 * This SDK provides types and utilities for building GameAP frontend plugins.
 *
 * @example
 * ```typescript
 * import type { PluginDefinition } from '@gameap/plugin-sdk';
 * import { usePluginContext, useServer } from '@gameap/plugin-sdk';
 * import MyComponent from './components/MyComponent.vue';
 *
 * export const myPlugin: PluginDefinition = {
 *   id: 'my-plugin',
 *   name: 'My Plugin',
 *   version: '1.0.0',
 *   apiVersion: '1.0',
 *   routes: [
 *     { path: '/', name: 'index', component: MyComponent }
 *   ]
 * };
 * ```
 *
 * @packageDocumentation
 */

// Type exports
export type {
    PluginDefinition,
    PluginRoute,
    PluginMenuItem,
    PluginSlotComponent,
    PluginHomeButton,
    PluginFileEditor,
    SlotName,
    ServerData,
    UserData,
    PluginContext,
    PluginRouteInfo,
    ServerTabProps,
    DashboardWidgetProps,
    ServerControlProps,
    ServersListActionProps,
    ChromeSlotProps,
    SidebarSectionProps,
    ProfileSlotProps,
    AdminUserInfoProps,
    UserEditFormData,
    AdminUserEditBlockProps,
    AdminNodeEditBlockProps,
    AdminServerEditBlockProps,
    AdminGameEditBlockProps,
    AdminModEditBlockProps,
    EditorContentType,
    EditorMatchRules,
    EditorMenuGroup,
    FileEditorProps,
} from './types';

// Context hooks
export {
    usePluginContext,
    useServer,
    useServerId,
    useServerAbilities,
    useCurrentUser,
    useIsAdmin,
    useIsAuthenticated,
    usePluginRoute,
    usePluginId,
} from './context';

// i18n
export { usePluginTrans, providePluginTrans } from './i18n';
export type { PluginI18nContext } from './i18n';

// Re-export Vue utilities that plugins commonly need
export { defineComponent, ref, computed, watch, onMounted, onUnmounted } from 'vue';

// Re-export @gameap/ui components for plugin convenience
// Plugins can also import directly from '@gameap/ui'
export {
    GBreadcrumbs,
    GCard,
    GDataTable,
    GDeletableList,
    GDivider,
    GEmpty,
    GGameIcon,
    GIcon,
    GInput,
    GMenu,
    GMenuButton,
    GMenuItem,
    GMenuItems,
    GModal,
    GStatusBadge,
    GSwitch,
    GTable,
    Loading,
    Progressbar,
    registerIcons,
    iconRegistry,
    getIcon,
    hasIcon,
    defaultIconMap,
} from '@gameap/ui';
