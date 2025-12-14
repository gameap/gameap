import { inject, computed, type ComputedRef } from 'vue';
import type { PluginContext, ServerData, UserData, PluginRouteInfo } from './types';

const PLUGIN_CONTEXT_KEY = 'pluginContext';

/**
 * Inject and return the full plugin context.
 * Must be called within a component that has plugin context provided.
 *
 * @example
 * ```vue
 * <script setup>
 * import { usePluginContext } from '@gameap/plugin-sdk';
 *
 * const ctx = usePluginContext();
 * console.log(ctx.user.value.login);
 * </script>
 * ```
 */
export function usePluginContext(): PluginContext {
    const context = inject<PluginContext>(PLUGIN_CONTEXT_KEY);
    if (!context) {
        throw new Error(
            'Plugin context not available. ' +
            'Ensure your component is rendered within a plugin slot or route.'
        );
    }
    return context;
}

/**
 * Get the current server data.
 * Returns null when not on a server page.
 *
 * @example
 * ```vue
 * <script setup>
 * import { useServer } from '@gameap/plugin-sdk';
 *
 * const server = useServer();
 * // server.value is ServerData | null
 * </script>
 * ```
 */
export function useServer(): ComputedRef<ServerData | null> {
    const ctx = usePluginContext();
    return computed(() => ctx.server.value.data);
}

/**
 * Get the current server ID.
 * Returns null when not on a server page.
 */
export function useServerId(): ComputedRef<number | null> {
    const ctx = usePluginContext();
    return computed(() => ctx.server.value.id);
}

/**
 * Get the current user's abilities for the server.
 */
export function useServerAbilities(): ComputedRef<string[]> {
    const ctx = usePluginContext();
    return computed(() => ctx.server.value.abilities);
}

/**
 * Get the current user information.
 *
 * @example
 * ```vue
 * <script setup>
 * import { useCurrentUser } from '@gameap/plugin-sdk';
 *
 * const user = useCurrentUser();
 * // user.value.isAdmin, user.value.login, etc.
 * </script>
 * ```
 */
export function useCurrentUser(): ComputedRef<UserData> {
    const ctx = usePluginContext();
    return computed(() => ctx.user.value);
}

/**
 * Check if the current user is an admin.
 */
export function useIsAdmin(): ComputedRef<boolean> {
    const ctx = usePluginContext();
    return computed(() => ctx.user.value.isAdmin);
}

/**
 * Check if the current user is authenticated.
 */
export function useIsAuthenticated(): ComputedRef<boolean> {
    const ctx = usePluginContext();
    return computed(() => ctx.user.value.isAuthenticated);
}

/**
 * Get the current plugin route information.
 */
export function usePluginRoute(): ComputedRef<PluginRouteInfo> {
    const ctx = usePluginContext();
    return computed(() => ctx.route.value);
}

/**
 * Get the current plugin's ID from the route.
 */
export function usePluginId(): ComputedRef<string | null> {
    const ctx = usePluginContext();
    return computed(() => ctx.route.value.pluginId);
}
