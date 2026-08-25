<template>
  <div class="space-y-2" data-testid="i18n-editor">
    <p v-if="rows.length === 0" class="text-sm text-stone-500 dark:text-stone-400">
      {{ trans('games.translations_empty') }}
    </p>

    <div
      v-for="(row, index) in rows"
      :key="index"
      class="grid grid-cols-1 sm:grid-cols-[10rem_1fr_2rem] gap-2 sm:items-start"
    >
      <n-select
        :value="row.locale || null"
        :options="localeOptions"
        filterable
        tag
        size="small"
        :placeholder="trans('games.translations_locale')"
        :status="localeProblem(row) ? 'error' : undefined"
        @update:value="setLocale(index, $event)"
      />

      <div class="space-y-1">
        <n-input
          v-for="field in fields"
          :key="field.key"
          :value="row.values[field.key] ?? ''"
          :type="field.type === 'textarea' ? 'textarea' : 'text'"
          :autosize="field.type === 'textarea' ? { minRows: 2, maxRows: 6 } : undefined"
          :maxlength="field.maxlength"
          :placeholder="field.label"
          size="small"
          @update:value="setField(index, field.key, $event)"
        />
        <small v-if="localeProblem(row)" class="block text-red-500">
          {{ localeProblem(row) }}
        </small>
      </div>

      <n-button quaternary size="small" type="error" @click="removeRow(index)">
        <GIcon name="trash" />
      </n-button>
    </div>

    <n-button size="small" dashed @click="addRow">
      <GIcon name="add" class="mr-1" />
      {{ trans('games.translations_add') }}
    </n-button>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { NButton, NInput, NSelect } from 'naive-ui'
import { GIcon } from '@gameap/ui'
import { trans, getAvailableLanguages } from '@/i18n/i18n'
import { normalizeLocale } from '@/parts/gameModVars'

const props = defineProps({
  /**
   * Which translated fields this map carries, e.g.
   * [{key: 'info', label: 'Info', maxlength: 128}].
   */
  fields: {
    type: Array,
    required: true,
  },
})

/** {locale: {...fields}} or null */
const model = defineModel({ type: Object, default: null })

// The editor works on an ordered array so a half-typed locale key does not
// collapse two rows into one while the user is still typing.
const rows = ref([])

watch(model, (value) => {
  if (rowsMatchModel(value)) {
    return
  }

  rows.value = Object.entries(value || {}).map(([locale, values]) => ({
    locale,
    values: { ...values },
  }))
}, { immediate: true, deep: true })

// buildModel keys the payload by locale, so a locale entered twice silently
// discards every row but the last one. It is reported instead.
const duplicateLocales = computed(() => {
  const seen = new Set()
  const duplicates = new Set()

  for (const row of rows.value) {
    const locale = normalizeLocale(row.locale)
    if (!locale) {
      continue
    }

    if (seen.has(locale)) {
      duplicates.add(locale)
    }
    seen.add(locale)
  }

  return duplicates
})

function localeProblem(row) {
  const locale = normalizeLocale(row.locale)

  if (locale === 'en') {
    return trans('games.translations_en_hint')
  }

  if (duplicateLocales.value.has(locale)) {
    return trans('games.translations_duplicate')
  }

  return null
}

function rowsMatchModel(value) {
  return JSON.stringify(buildModel(rows.value)) === JSON.stringify(value ?? null)
}

// The seeded list makes ru/es/de one click away while `tag` still allows typing
// a regional code such as pt-br.
const localeOptions = computed(() =>
  getAvailableLanguages()
    .filter((language) => language.code !== 'en')
    .map((language) => ({
      label: `${language.native_name || language.name} (${language.code})`,
      value: language.code,
    })),
)

function buildModel(list) {
  const map = {}

  for (const row of list) {
    const locale = normalizeLocale(row.locale)
    if (!locale || locale === 'en') {
      continue
    }

    const values = {}
    for (const field of props.fields) {
      const text = String(row.values[field.key] ?? '').trim()
      if (text) {
        values[field.key] = text
      }
    }

    if (Object.keys(values).length > 0) {
      map[locale] = values
    }
  }

  return Object.keys(map).length > 0 ? map : null
}

function emitModel() {
  model.value = buildModel(rows.value)
}

function addRow() {
  rows.value.push({ locale: '', values: {} })
}

function removeRow(index) {
  rows.value.splice(index, 1)
  emitModel()
}

function setLocale(index, value) {
  rows.value[index].locale = normalizeLocale(value)
  emitModel()
}

function setField(index, key, value) {
  rows.value[index].values[key] = value
  emitModel()
}
</script>
