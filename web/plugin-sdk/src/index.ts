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
    EditorContentType,
    EditorMatchRules,
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
export { usePluginTrans } from './i18n';
export type { PluginI18nContext } from './i18n';

// Re-export Vue utilities that plugins commonly need
export { defineComponent, ref, computed, watch, onMounted, onUnmounted } from 'vue';
