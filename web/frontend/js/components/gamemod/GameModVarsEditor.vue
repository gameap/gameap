<template>
  <div class="space-y-3" data-testid="mod-vars-editor">
    <n-input
      v-if="items.length > 5"
      v-model:value="search"
      size="small"
      clearable
      :placeholder="trans('games.search_vars')"
      class="max-w-sm"
    />

    <n-collapse :expanded-names="expandedNames" @update:expanded-names="expandedNames = $event">
      <n-collapse-item
        v-for="entry in visibleItems"
        :key="entry.index"
        :name="entry.index"
        :data-testid="`mod-var-card-${entry.index}`"
      >
        <template #header>
          <span class="flex items-center gap-2 text-sm">
            <span class="font-mono">{{ entry.item.var || '—' }}</span>
            <n-tag size="tiny" round>{{ typeLabel(entry.item.type) }}</n-tag>
            <span class="text-stone-500 dark:text-stone-400 truncate">{{ entry.item.info }}</span>
            <!-- Marks a variable carrying options, rules or translations, so
                 nothing nested is invisible from the collapsed list. -->
            <span
              v-if="hasAdvancedSettings(entry.item)"
              class="w-1.5 h-1.5 rounded-full bg-sky-500"
              :title="trans('games.advanced')"
            />
          </span>
        </template>

        <template #header-extra>
          <span class="flex items-center gap-1" @click.stop>
            <n-button
              quaternary
              size="tiny"
              :disabled="entry.index === 0"
              :title="trans('games.move_up')"
              @click="move(entry.index, -1)"
            >
              <GIcon name="arrow-up" />
            </n-button>
            <n-button
              quaternary
              size="tiny"
              :disabled="entry.index === items.length - 1"
              :title="trans('games.move_down')"
              @click="move(entry.index, 1)"
            >
              <GIcon name="arrow-down" />
            </n-button>
            <n-button
              quaternary
              size="tiny"
              type="error"
              :title="trans('main.delete')"
              @click="removeItem(entry.index)"
            >
              <GIcon name="trash" />
            </n-button>
          </span>
        </template>

        <div class="space-y-3">
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <n-form-item :label="trans('games.var')" :show-feedback="false">
              <n-input
                :value="entry.item.var"
                :maxlength="32"
                :status="nameIsSuspicious(entry.item.var) ? 'warning' : undefined"
                @update:value="setField(entry.index, 'var', $event)"
              />
            </n-form-item>

            <n-form-item :label="trans('games.info')" :show-feedback="false">
              <n-input
                :value="entry.item.info"
                :maxlength="128"
                @update:value="setField(entry.index, 'info', $event)"
              />
            </n-form-item>
          </div>

          <p class="text-xs text-stone-500 dark:text-stone-400">
            {{ trans('games.var_name_hint') }}
          </p>

          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <n-form-item :label="trans('games.var_type')" :show-feedback="false">
              <n-select
                :value="entry.item.type || DEFAULT_VAR_TYPE"
                :options="typeOptions"
                @update:value="setType(entry.index, $event)"
              />
            </n-form-item>

            <n-form-item :label="trans('games.default')" :show-feedback="false">
              <!-- A typed widget only where the constraint is real: a bool must
                   pick one of its two values and a select one of its options.
                   Everything else stays a plain input, since the schema stores
                   the default as a string anyway. -->
              <VarValueField
                v-if="usesTypedDefault(entry.item)"
                :definition="definitionFor(entry.item)"
                :value="defaultAsValue(entry.item)"
                @update:value="setDefaultFromValue(entry.index, $event)"
              />
              <n-input
                v-else
                :value="String(entry.item.default ?? '')"
                :maxlength="64"
                @update:value="setField(entry.index, 'default', $event)"
              />
            </n-form-item>
          </div>

          <n-form-item :label="trans('games.description')" :show-feedback="false">
            <n-input
              :value="entry.item.description ?? ''"
              type="textarea"
              :autosize="{ minRows: 2, maxRows: 6 }"
              :maxlength="1000"
              @update:value="setField(entry.index, 'description', $event)"
            />
          </n-form-item>

          <div class="flex items-center gap-2">
            <n-switch
              :value="entry.item.admin_var === true"
              size="small"
              @update:value="setField(entry.index, 'admin_var', $event === true)"
            />
            <span class="text-sm">{{ trans('games.admin_var') }}</span>
          </div>

          <template v-if="(entry.item.type || DEFAULT_VAR_TYPE) === 'select'">
            <div class="flex items-center gap-2">
              <n-switch
                :value="entry.item.allow_custom === true"
                size="small"
                @update:value="setField(entry.index, 'allow_custom', $event === true)"
              />
              <span class="text-sm">{{ trans('games.var_allow_custom') }}</span>
            </div>

            <n-divider class="!my-2">{{ trans('games.var_options') }}</n-divider>
            <VarOptionsEditor
              :model-value="entry.item.options ?? []"
              @update:model-value="setField(entry.index, 'options', $event)"
            />
          </template>

          <template v-if="entry.item.type === 'bool'">
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <n-form-item :label="trans('games.var_true_value')" :show-feedback="false">
                <n-input
                  :value="entry.item.true_value ?? '1'"
                  :maxlength="64"
                  @update:value="setField(entry.index, 'true_value', $event)"
                />
              </n-form-item>
              <n-form-item :label="trans('games.var_false_value')" :show-feedback="false">
                <n-input
                  :value="entry.item.false_value ?? '0'"
                  :maxlength="64"
                  @update:value="setField(entry.index, 'false_value', $event)"
                />
              </n-form-item>
            </div>
          </template>

          <n-divider class="!my-2">{{ trans('games.var_rules') }}</n-divider>
          <VarRulesEditor
            :type="entry.item.type || DEFAULT_VAR_TYPE"
            :allow-custom="entry.item.allow_custom === true"
            :model-value="entry.item.rules ?? null"
            @update:model-value="setField(entry.index, 'rules', $event)"
          />

          <n-divider class="!my-2">{{ trans('games.translations') }}</n-divider>
          <I18nEditor
            :fields="i18nFields"
            :model-value="entry.item.i18n ?? null"
            @update:model-value="setField(entry.index, 'i18n', $event)"
          />

          <p
            v-for="issue in varDefinitionIssues(entry.item)"
            :key="issue"
            class="text-sm text-red-500"
          >
            {{ issue }}
          </p>
        </div>
      </n-collapse-item>
    </n-collapse>

    <n-button size="small" dashed data-testid="mod-var-add" @click="addItem">
      <GIcon name="add" class="mr-1" />
      {{ trans('games.add_var') }}
    </n-button>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import { NButton, NCollapse, NCollapseItem, NDivider, NFormItem, NInput, NSelect, NSwitch, NTag } from 'naive-ui'
import { GIcon } from '@gameap/ui'
import { trans } from '@/i18n/i18n'
import {
  DEFAULT_VAR_TYPE,
  VAR_NAME_PATTERN,
  VAR_NAME_MAX_LENGTH,
  VAR_TYPES,
  coerceValue,
  hasAdvancedSettings,
  normalizeVarDefinition,
  varDefinitionIssues,
} from '@/parts/gameModVars'
import VarValueField from '@/components/input/VarValueField.vue'
import VarOptionsEditor from '@/components/gamemod/VarOptionsEditor.vue'
import VarRulesEditor from '@/components/gamemod/VarRulesEditor.vue'
import I18nEditor from '@/components/gamemod/I18nEditor.vue'

/**
 * The raw snake_case variable rows, matching the API byte for byte. The
 * camelCase shape produced by normalizeVarDefinition is for rendering only and
 * is never turned back into a payload here.
 */
const items = defineModel({ type: Array, default: () => [] })

const expandedNames = ref([])
const search = ref('')

const typeOptions = VAR_TYPES.map((type) => ({
  label: trans(`games.var_type_${type}`),
  value: type,
}))

const i18nFields = [
  { key: 'info', label: trans('games.info'), maxlength: 128 },
  { key: 'description', label: trans('games.description'), type: 'textarea', maxlength: 1000 },
]

const visibleItems = computed(() => {
  const entries = items.value.map((item, index) => ({ item, index }))
  const query = search.value.trim().toLowerCase()

  if (!query) {
    return entries
  }

  return entries.filter(({ item }) =>
    String(item.var ?? '').toLowerCase().includes(query)
    || String(item.info ?? '').toLowerCase().includes(query),
  )
})

function typeLabel(type) {
  return trans(`games.var_type_${type || DEFAULT_VAR_TYPE}`)
}

// The catalog schema is stricter than the panel, which also hosts imported
// Pelican eggs; a name it would reject is a warning, not an error.
function nameIsSuspicious(name) {
  const value = String(name ?? '').trim()

  return value !== ''
    && (value.length > VAR_NAME_MAX_LENGTH || !VAR_NAME_PATTERN.test(value))
}

function usesTypedDefault(item) {
  return item.type === 'bool' || item.type === 'select'
}

function definitionFor(item) {
  return normalizeVarDefinition(item)
}

function defaultAsValue(item) {
  return coerceValue(definitionFor(item), item.default ?? '')
}

function setDefaultFromValue(index, value) {
  const item = items.value[index]

  if (item.type === 'bool') {
    setField(index, 'default', value === true ? (item.true_value ?? '1') : (item.false_value ?? '0'))

    return
  }

  setField(index, 'default', value === null || value === undefined ? '' : String(value))
}

function setField(index, key, value) {
  const next = [...items.value]
  next[index] = { ...next[index], [key]: value }
  items.value = next
}

function setType(index, type) {
  // The incompatible leftovers are dropped at save time by denormalizeVar; an
  // out-of-place default is kept and flagged instead, because losing a value on
  // a mis-click is worse than a warning.
  setField(index, 'type', type)
}

function addItem() {
  items.value = [
    ...items.value,
    { var: '', default: '', info: '', admin_var: false, type: DEFAULT_VAR_TYPE },
  ]
  expandedNames.value = [...expandedNames.value, items.value.length - 1]
}

function removeItem(index) {
  const next = [...items.value]
  next.splice(index, 1)
  items.value = next
  expandedNames.value = []
}

function move(index, offset) {
  const target = index + offset
  if (target < 0 || target >= items.value.length) {
    return
  }

  const next = [...items.value]
  const [moved] = next.splice(index, 1)
  next.splice(target, 0, moved)
  items.value = next
  expandedNames.value = []
}
</script>
