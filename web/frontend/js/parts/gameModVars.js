import { trans, getCurrentLanguage } from '@/i18n/i18n';

/**
 * Game mod variable helpers.
 *
 * A variable arrives in two shapes: the server settings endpoint calls the
 * fields `name`/`label`, the game mod endpoint calls them `var`/`info`, and the
 * `i18n` map always keys its entries by `info`. `normalizeVarDefinition` folds
 * both into one shape for rendering; the payload sent back is always rebuilt
 * from the raw object by `denormalizeVar`, so future schema keys survive an
 * edit even though the renderer knows nothing about them.
 */

export const VAR_TYPES = ['string', 'text', 'int', 'float', 'bool', 'select', 'password'];
export const DEFAULT_VAR_TYPE = 'string';
export const TEXTUAL_TYPES = ['string', 'text', 'password'];
export const NUMERIC_TYPES = ['int', 'float'];

export const VAR_NAME_PATTERN = /^[a-z][a-z0-9_]*$/;
export const VAR_NAME_MAX_LENGTH = 32;
export const VAR_PATTERN_MAX_LENGTH = 512;

// The two states a bool variable carries when nothing was customized.
export const DEFAULT_VAR_TRUE_VALUE = '1';
export const DEFAULT_VAR_FALSE_VALUE = '0';

// Go's RE2 has neither lookarounds nor backreferences, so a pattern using them
// would be accepted here and rejected by the panel.
const RE2_UNSUPPORTED_PATTERN = /\(\?[=!<]|\\[1-9]/;

const TRUTHY_STRINGS = new Set(['1', 'true', 'on', 'yes', 'y']);

export function normalizeLocale(locale) {
    return String(locale ?? '').trim().toLowerCase().replace(/_/g, '-');
}

/**
 * Resolve a translated field from a data-carried `i18n` map.
 *
 * Falls back from the exact locale to the bare language, then to any regional
 * variant of that language, then to the base field. The last step matters:
 * `window.gameapLang` is a bare two-letter code, so a `pt-br`-only translation
 * would otherwise never be shown to anybody.
 *
 * @param {object} entry object carrying an `i18n` map plus the base field
 * @param {string} field key inside the i18n map: info, description or label
 * @param {string} [locale] defaults to the current interface language
 * @param {string} [baseField] key on `entry` holding the English text
 */
export function resolveI18n(entry, field, locale = getCurrentLanguage(), baseField = field) {
    const base = entry?.[baseField] ?? '';

    const map = entry?.i18n;
    if (!map || typeof map !== 'object' || Array.isArray(map)) {
        return base;
    }

    const want = normalizeLocale(locale);
    if (!want || want === 'en') {
        return base;
    }

    // Indexed defensively: the catalog stores lowercase keys, but an admin can
    // type "pt-BR" into the translations editor.
    const byLocale = new Map(
        Object.entries(map).map(([key, value]) => [normalizeLocale(key), value]),
    );

    const exact = byLocale.get(want)?.[field];
    if (exact) {
        return exact;
    }

    const language = want.split('-')[0];

    if (language !== want) {
        const languageOnly = byLocale.get(language)?.[field];
        if (languageOnly) {
            return languageOnly;
        }
    }

    for (const [key, value] of byLocale) {
        if (key.split('-')[0] === language && value?.[field]) {
            return value[field];
        }
    }

    return base;
}

export const localizedLabel = (definition, locale) =>
    resolveI18n(definition, 'info', locale, 'label');

export const localizedDescription = (definition, locale) =>
    resolveI18n(definition, 'description', locale);

export const localizedOptionLabel = (option, locale) =>
    resolveI18n(option, 'label', locale) || option.value;

export function normalizeOptions(options) {
    if (!Array.isArray(options)) {
        return [];
    }

    return options.map((option) =>
        typeof option === 'string'
            ? { value: option, label: '', i18n: null }
            : {
                value: option?.value ?? '',
                label: option?.label ?? '',
                i18n: option?.i18n ?? null,
            },
    );
}

const numberOrNull = (value) => {
    if (value === null || value === undefined || value === '') {
        return null;
    }

    const parsed = Number(value);

    return Number.isNaN(parsed) ? null : parsed;
};

/**
 * Fold either payload shape into the object the field renderer consumes.
 * Strictly one-way: never turn the result back into a payload.
 */
export function normalizeVarDefinition(raw) {
    const type = VAR_TYPES.includes(raw?.type) ? raw.type : DEFAULT_VAR_TYPE;
    const rules = raw?.rules ?? {};

    return {
        name: raw?.name ?? raw?.var ?? '',
        type,
        label: raw?.label ?? raw?.info ?? raw?.name ?? raw?.var ?? '',
        description: raw?.description ?? '',
        i18n: raw?.i18n ?? null,
        adminVar: raw?.admin_var === true,
        default: raw?.default ?? null,
        options: normalizeOptions(raw?.options),
        allowCustom: raw?.allow_custom === true,
        trueValue: raw?.true_value ?? DEFAULT_VAR_TRUE_VALUE,
        falseValue: raw?.false_value ?? DEFAULT_VAR_FALSE_VALUE,
        rules: {
            required: rules.required === true,
            min: numberOrNull(rules.min),
            max: numberOrNull(rules.max),
            minLength: numberOrNull(rules.min_length),
            maxLength: numberOrNull(rules.max_length),
            pattern: rules.pattern || null,
        },
    };
}

/** A select with allow_custom takes free text, so the length rules apply to it. */
export const acceptsFreeText = (definition) =>
    TEXTUAL_TYPES.includes(definition.type)
    || (definition.type === 'select' && definition.allowCustom);

export const isBlankValue = (value) =>
    value === undefined || value === null || (typeof value === 'string' && value.trim() === '');

/** Convert a stored value into what the widget for this type binds to. */
export function coerceValue(definition, raw) {
    switch (definition.type) {
        case 'bool':
            if (typeof raw === 'boolean') {
                return raw;
            }
            if (raw === null || raw === undefined) {
                return false;
            }
            // A declared true/false value wins over the generic spellings so a
            // variable with true_value "on" round-trips exactly.
            if (String(raw) === String(definition.trueValue)) {
                return true;
            }
            if (String(raw) === String(definition.falseValue)) {
                return false;
            }

            return TRUTHY_STRINGS.has(String(raw).trim().toLowerCase());

        case 'int': {
            if (isBlankValue(raw)) {
                return null;
            }
            const parsed = Number.parseInt(raw, 10);

            return Number.isNaN(parsed) ? null : parsed;
        }

        case 'float': {
            if (isBlankValue(raw)) {
                return null;
            }
            const parsed = Number.parseFloat(raw);

            return Number.isNaN(parsed) ? null : parsed;
        }

        default:
            return raw === null || raw === undefined ? '' : String(raw);
    }
}

/** Convert a widget value into the JSON the API expects for this type. */
export function serializeValue(definition, value) {
    switch (definition.type) {
        case 'bool':
            return value === true;
        case 'int':
        case 'float':
            return isBlankValue(value) ? null : Number(value);
        default:
            return value === null || value === undefined ? '' : String(value);
    }
}

export function isRe2Compatible(pattern) {
    return !RE2_UNSUPPORTED_PATTERN.test(pattern);
}

export function compilePattern(pattern) {
    // Mirrors the panel's own limit: a longer pattern is rejected on save, and
    // compiling an unbounded one costs the browser more than it is worth.
    if (String(pattern ?? '').length > VAR_PATTERN_MAX_LENGTH) {
        return null;
    }

    try {
        // The whole value must match, exactly as the panel anchors it.
        return new RegExp(`^(?:${pattern})$`);
    } catch {
        return null;
    }
}

/**
 * Build the naive-ui rules for one variable so the form rejects locally what the
 * panel would reject with a 422.
 *
 * Deliberately does not reuse `requiredValidator` from parts/validators.js: that
 * one is written as `if (!value)` and would reject `false` and `0`, making a
 * switch and a zero-valued number field impossible to save.
 */
export function buildVarRules(definition, label) {
    const rules = [];
    const constraints = definition.rules;

    if (constraints.required) {
        rules.push({
            required: true,
            trigger: ['blur', 'change'],
            validator: (_rule, value) => {
                if (isBlankValue(value)) {
                    return new Error(trans('validation.required', { attribute: label }));
                }
            },
        });
    }

    if (NUMERIC_TYPES.includes(definition.type)) {
        rules.push({
            type: 'number',
            trigger: ['blur', 'change'],
            validator: (_rule, value) => {
                if (value === null || value === undefined) {
                    // Emptiness is the required rule's business.
                    return;
                }

                if (definition.type === 'int' && !Number.isInteger(value)) {
                    return new Error(trans('validation.integer', { attribute: label }));
                }

                const { min, max } = constraints;

                if (min !== null && max !== null && (value < min || value > max)) {
                    return new Error(
                        trans('validation.between.numeric', { attribute: label, min, max }),
                    );
                }
                if (min !== null && value < min) {
                    return new Error(trans('validation.min.numeric', { attribute: label, min }));
                }
                if (max !== null && value > max) {
                    return new Error(trans('validation.max.numeric', { attribute: label, max }));
                }
            },
        });
    }

    if (acceptsFreeText(definition)) {
        rules.push({
            trigger: ['blur', 'change'],
            validator: (_rule, value) => {
                const text = value === null || value === undefined ? '' : String(value);
                if (text === '') {
                    // Rules apply to non-empty values only.
                    return;
                }

                const { minLength, maxLength, pattern } = constraints;

                if (minLength !== null && text.length < minLength) {
                    return new Error(
                        trans('validation.min.string', { attribute: label, min: minLength }),
                    );
                }
                if (maxLength !== null && text.length > maxLength) {
                    return new Error(
                        trans('validation.max.string', { attribute: label, max: maxLength }),
                    );
                }
                if (pattern) {
                    const compiled = compilePattern(pattern);
                    // An uncompilable pattern is left to the panel rather than
                    // blocking the save on a client-side quirk.
                    if (compiled && !compiled.test(text)) {
                        return new Error(trans('validation.regex', { attribute: label }));
                    }
                }
            },
        });
    }

    if (definition.type === 'select' && !definition.allowCustom && definition.options.length > 0) {
        rules.push({
            trigger: ['change'],
            validator: (_rule, value) => {
                if (isBlankValue(value)) {
                    return;
                }

                if (!definition.options.some((option) => option.value === String(value))) {
                    return new Error(trans('validation.in', { attribute: label }));
                }
            },
        });
    }

    return rules;
}

function compactI18n(map, fields) {
    if (!map || typeof map !== 'object' || Array.isArray(map)) {
        return null;
    }

    const compacted = {};

    for (const [rawLocale, entry] of Object.entries(map)) {
        const locale = normalizeLocale(rawLocale);
        // English lives in the base fields, so the schema forbids it here.
        if (!locale || locale === 'en' || !entry || typeof entry !== 'object') {
            continue;
        }

        const translated = {};
        for (const field of fields) {
            const text = String(entry[field] ?? '').trim();
            if (text) {
                translated[field] = text;
            }
        }

        if (Object.keys(translated).length > 0) {
            compacted[locale] = translated;
        }
    }

    return Object.keys(compacted).length > 0 ? compacted : null;
}

function compactRules(rules, type, allowCustom) {
    if (!rules || typeof rules !== 'object') {
        return null;
    }

    const compacted = {};

    // `required: false` carries no information, so only `true` is emitted.
    if (rules.required === true) {
        compacted.required = true;
    }

    if (NUMERIC_TYPES.includes(type)) {
        for (const key of ['min', 'max']) {
            const value = numberOrNull(rules[key]);
            if (value !== null) {
                compacted[key] = value;
            }
        }
    }

    const freeText = TEXTUAL_TYPES.includes(type) || (type === 'select' && allowCustom);

    if (freeText) {
        for (const key of ['min_length', 'max_length']) {
            const value = numberOrNull(rules[key]);
            if (value !== null) {
                compacted[key] = value;
            }
        }

        const pattern = String(rules.pattern ?? '').trim();
        if (pattern) {
            compacted.pattern = pattern;
        }
    }

    // The schema requires a non-empty rules object, so an empty one is dropped.
    return Object.keys(compacted).length > 0 ? compacted : null;
}

/**
 * Build the API payload for one variable definition from the raw editor row.
 * Fields that do not apply to the chosen type are dropped, and an option with
 * nothing but a value collapses back to the schema's plain-string shorthand.
 */
export function denormalizeVar(raw) {
    const type = VAR_TYPES.includes(raw?.type) ? raw.type : DEFAULT_VAR_TYPE;

    const payload = {
        var: String(raw?.var ?? '').trim(),
        default: raw?.default ?? '',
        info: String(raw?.info ?? '').trim(),
        admin_var: raw?.admin_var === true,
    };

    if (type !== DEFAULT_VAR_TYPE) {
        payload.type = type;
    }

    const description = String(raw?.description ?? '').trim();
    if (description) {
        payload.description = description;
    }

    if (type === 'select') {
        const options = normalizeOptions(raw?.options)
            .filter((option) => String(option.value).trim() !== '')
            .map((option) => {
                const value = String(option.value).trim();
                const label = String(option.label ?? '').trim();
                const i18n = compactI18n(option.i18n, ['label']);

                if ((!label || label === value) && !i18n) {
                    return value;
                }

                const object = { value };
                if (label && label !== value) {
                    object.label = label;
                }
                if (i18n) {
                    object.i18n = i18n;
                }

                return object;
            });

        if (options.length > 0) {
            payload.options = options;
        }

        if (raw?.allow_custom === true) {
            payload.allow_custom = true;
        }
    }

    if (type === 'bool') {
        // Always emitted: an empty false_value is a meaningful flag-style value
        // that any "drop when empty" rule would eat.
        payload.true_value = String(raw?.true_value ?? DEFAULT_VAR_TRUE_VALUE);
        payload.false_value = String(raw?.false_value ?? DEFAULT_VAR_FALSE_VALUE);
    }

    const rules = compactRules(raw?.rules, type, raw?.allow_custom === true);
    if (rules) {
        payload.rules = rules;
    }

    const i18n = compactI18n(raw?.i18n, ['info', 'description']);
    if (i18n) {
        payload.i18n = i18n;
    }

    return payload;
}

export function denormalizeFastRcon(raw) {
    const payload = {
        info: String(raw?.info ?? '').trim(),
        command: String(raw?.command ?? '').trim(),
    };

    const i18n = compactI18n(raw?.i18n, ['info']);
    if (i18n) {
        payload.i18n = i18n;
    }

    return payload;
}

/** Problems the admin editor surfaces before the payload leaves the browser. */
export function varDefinitionIssues(raw) {
    const issues = [];
    const type = VAR_TYPES.includes(raw?.type) ? raw.type : DEFAULT_VAR_TYPE;
    const name = String(raw?.var ?? '').trim();

    if (name && (name.length > VAR_NAME_MAX_LENGTH || !VAR_NAME_PATTERN.test(name))) {
        issues.push(trans('games.var_name_invalid'));
    }

    if (type === 'select') {
        const options = normalizeOptions(raw?.options)
            .map((option) => String(option.value).trim())
            .filter((value) => value !== '');

        if (options.length === 0) {
            issues.push(trans('games.var_options_required'));
        }

        if (new Set(options).size !== options.length) {
            issues.push(trans('games.var_options_duplicate'));
        }
    }

    if (type === 'bool') {
        const trueValue = String(raw?.true_value ?? DEFAULT_VAR_TRUE_VALUE);
        const falseValue = String(raw?.false_value ?? DEFAULT_VAR_FALSE_VALUE);
        const defaultValue = String(raw?.default ?? '');

        if (defaultValue && defaultValue !== trueValue && defaultValue !== falseValue) {
            issues.push(trans('games.var_bool_default_hint'));
        }
    }

    const pattern = String(raw?.rules?.pattern ?? '').trim();
    if (pattern) {
        if (!compilePattern(pattern)) {
            issues.push(trans('games.var_rule_pattern_invalid'));
        } else if (!isRe2Compatible(pattern)) {
            issues.push(trans('games.var_rule_pattern_unsupported'));
        }
    }

    return issues;
}

/** True when a variable carries anything beyond the four basic fields. */
export function hasAdvancedSettings(raw) {
    const type = VAR_TYPES.includes(raw?.type) ? raw.type : DEFAULT_VAR_TYPE;

    return Boolean(
        String(raw?.description ?? '').trim()
        || (type === 'select' && (raw?.options?.length || raw?.allow_custom))
        // The payload always carries true_value/false_value for a bool, so only a
        // value that differs from the default counts as an advanced setting.
        || (type === 'bool' && (
            String(raw?.true_value ?? DEFAULT_VAR_TRUE_VALUE) !== DEFAULT_VAR_TRUE_VALUE
            || String(raw?.false_value ?? DEFAULT_VAR_FALSE_VALUE) !== DEFAULT_VAR_FALSE_VALUE
        ))
        || compactRules(raw?.rules, type, raw?.allow_custom === true)
        || compactI18n(raw?.i18n, ['info', 'description']),
    );
}
