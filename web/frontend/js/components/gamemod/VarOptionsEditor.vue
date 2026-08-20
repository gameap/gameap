<template>
  <div class="space-y-2" data-testid="var-options-editor">
    <div
      v-for="(row, index) in rows"
      :key="index"
      class="space-y-1"
    >
      <div class="grid grid-cols-1 sm:grid-cols-[1fr_1fr_2rem_2rem] gap-2 sm:items-center">
        <n-input
          :value="row.value"
          size="small"
          :maxlength="64"
          :placeholder="trans('games.var_option_value')"
          :status="duplicateValues.has(row.value) ? 'error' : undefined"
          @update:value="setValue(index, $event)"
        />
        <n-input
          :value="row.label"
          size="small"
          :maxlength="128"
          :placeholder="row.value || trans('games.var_option_label')"
          @update:value="setLabel(index, $event)"
        />
        <n-button
          quaternary
          size="small"
          :type="row.i18n ? 'primary' : 'default'"
          :title="trans('games.translations')"
          @click="toggleTranslations(index)"
        >
          <GIcon name="languages" />
        </n-button>
        <n-button quaternary size="small" type="error" @click="removeRow(index)">
          <GIcon name="trash" />
        </n-button>
      </div>

      <!-- Inline sub-row rather than a nested modal: option translations are
           rare and a modal on top of a modal is fragile to focus and z-index. -->
      <div v-if="expanded.has(index)" class="pl-2 border-l-2 border-stone-200 dark:border-stone-700">
        <I18nEditor
          :fields="[{ key: 'label', label: trans('games.var_option_label'), maxlength: 128 }]"
          :model-value="row.i18n"
          @update:model-value="setTranslations(index, $event)"
        />
      </div>
    </div>

    <p v-if="rows.length === 0" class="text-sm text-red-500">
      {{ trans('games.var_options_required') }}
    </p>
    <p v-else-if="duplicateValues.size > 0" class="text-sm text-red-500">
      {{ trans('games.var_options_duplicate') }}
    </p>

    <n-button size="small" dashed @click="addRow">
      <GIcon name="add" class="mr-1" />
      {{ trans('games.var_option_add') }}
    </n-button>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { NButton, NInput } from 'naive-ui'
import { GIcon } from '@gameap/ui'
import { trans } from '@/i18n/i18n'
import { normalizeOptions } from '@/parts/gameModVars'
import I18nEditor from '@/components/gamemod/I18nEditor.vue'

/** The raw options array, either shape allowed by the schema. */
const model = defineModel({ type: Array, default: () => [] })

const rows = ref([])
const expanded = ref(new Set())

watch(model, (value) => {
  if (JSON.stringify(buildModel(rows.value)) === JSON.stringify(value ?? [])) {
    return
  }

  rows.value = normalizeOptions(value)
}, { immediate: true, deep: true })

const duplicateValues = computed(() => {
  const seen = new Set()
  const duplicates = new Set()

  for (const row of rows.value) {
    const value = String(row.value).trim()
    if (!value) {
      continue
    }

    if (seen.has(value)) {
      duplicates.add(value)
    }
    seen.add(value)
  }

  return duplicates
})

// The shorthand collapse happens at save time in denormalizeVar, never on a
// keystroke: rewriting a row while the cursor sits in it is unusable.
function buildModel(list) {
  return list.map((row) => ({
    value: row.value,
    label: row.label,
    i18n: row.i18n,
  }))
}

function emitModel() {
  model.value = buildModel(rows.value)
}

function addRow() {
  rows.value.push({ value: '', label: '', i18n: null })
  emitModel()
}

function removeRow(index) {
  rows.value.splice(index, 1)
  expanded.value.delete(index)
  emitModel()
}

function setValue(index, value) {
  rows.value[index].value = value
  emitModel()
}

function setLabel(index, label) {
  rows.value[index].label = label
  emitModel()
}

function setTranslations(index, i18n) {
  rows.value[index].i18n = i18n
  emitModel()
}

function toggleTranslations(index) {
  const next = new Set(expanded.value)

  if (next.has(index)) {
    next.delete(index)
  } else {
    next.add(index)
  }

  expanded.value = next
}
</script>
