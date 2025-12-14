import { inject } from 'vue';

const PLUGIN_I18N_KEY = 'pluginI18n';

export interface PluginI18nContext {
    trans: (key: string, params?: Record<string, string | number>) => string;
    locale: string;
}

/**
 * Hook to access plugin translations.
 *
 * @example
 * ```vue
 * <script setup>
 * import { usePluginTrans } from '@gameap/plugin-sdk';
 *
 * const { trans } = usePluginTrans();
 * </script>
 *
 * <template>
 *     <p>{{ trans('greeting', { name: 'World' }) }}</p>
 * </template>
 * ```
 */
export function usePluginTrans(): PluginI18nContext {
    const ctx = inject<PluginI18nContext>(PLUGIN_I18N_KEY);
    if (ctx) {
        return ctx;
    }

    return {
        trans: (key: string) => key,
        locale: 'en'
    };
}

/**
 * Creates a plugin i18n context for providing translations.
 * Used internally by PluginRouteWrapper.
 */
export function createPluginI18n(
    translations: Record<string, Record<string, string>> | undefined,
): PluginI18nContext {
    const locale = (window as unknown as { gameapLang?: string }).gameapLang || 'en';

    const trans = (key: string, params?: Record<string, string | number>): string => {
        const langTranslations = translations?.[locale] || translations?.['en'] || {};
        let value = langTranslations[key] ?? key;

        if (params) {
            Object.entries(params).forEach(([paramKey, paramVal]) => {
                value = value.replace(`:${paramKey}`, String(paramVal));
            });
        }

        return value;
    };

    return { trans, locale };
}

export const PLUGIN_I18N_INJECT_KEY = PLUGIN_I18N_KEY;
