<template>
  <div class="space-y-2" data-testid="server-vars-editor">
    <div
      v-for="(row, index) in rows"
      :key="index"
      class="grid grid-cols-1 sm:grid-cols-[1fr_2fr_2rem] gap-2 sm:items-center"
    >
      <n-input
        :value="row.key"
        size="small"
        :placeholder="trans('labels.key')"
        @update:value="setKey(index, $event)"
      />

      <!-- A key that names a game mod variable gets that variable's widget; a
           free-form key stays a plain input, since it is a low-level override
           the panel knows nothing about. -->
      <VarValueField
        v-if="definitionFor(row.key)"
        :definition="definitionFor(row.key)"
        :value="typedValue(row)"
        size="small"
        @update:value="setTypedValue(index, $event)"
      />
      <n-input
        v-else
        :value="row.value"
        size="small"
        :placeholder="trans('labels.the_value')"
        @update:value="setValue(index, $event)"
      />

      <n-button quaternary size="small" type="error" @click="removeRow(index)">
        <GIcon name="trash" />
      </n-button>
    </div>

    <n-button size="small" dashed @click="addRow">
      <GIcon name="add" class="mr-1" />
      {{ trans('main.add') }}
    </n-button>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { NButton, NInput } from 'naive-ui'
import { GIcon } from '@gameap/ui'
import { trans } from '@/i18n/i18n'
import { coerceValue, normalizeVarDefinition, serializeValue } from '@/parts/gameModVars'
import VarValueField from '@/components/input/VarValueField.vue'

const props = defineProps({
  /** The raw game mod variables of the server's mod. */
  vars: {
    type: Array,
    default: () => [],
  },
})

/** Rows of {key, value}, the same contract InputManyList used here before. */
const rows = defineModel({ type: Array, default: () => [] })

const definitions = computed(() => {
  const map = new Map()

  for (const raw of props.vars) {
    const definition = normalizeVarDefinition(raw)
    map.set(definition.name, definition)
  }

  return map
})

function definitionFor(key) {
  return definitions.value.get(key) ?? null
}

function typedValue(row) {
  const definition = definitionFor(row.key)

  return definition ? coerceValue(definition, row.value) : row.value
}

function replaceRow(index, patch) {
  const next = [...rows.value]
  next[index] = { ...next[index], ...patch }
  rows.value = next
}

function setKey(index, key) {
  replaceRow(index, { key })
}

function setValue(index, value) {
  replaceRow(index, { value })
}

// server.vars is a map of strings, so a typed widget value is converted back
// before it is stored.
function setTypedValue(index, value) {
  const definition = definitionFor(rows.value[index].key)
  const serialized = definition ? serializeValue(definition, value) : value

  replaceRow(index, { value: serialized === null ? '' : String(serialized) })
}

function addRow() {
  rows.value = [...rows.value, { key: '', value: '' }]
}

function removeRow(index) {
  const next = [...rows.value]
  next.splice(index, 1)
  rows.value = next
}
</script>
