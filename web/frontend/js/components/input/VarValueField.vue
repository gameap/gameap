<template>
  <n-input
    v-if="definition.type === 'text'"
    :value="value ?? ''"
    type="textarea"
    :autosize="{ minRows: 3, maxRows: 12 }"
    :maxlength="maxLength"
    :placeholder="placeholder"
    :disabled="disabled"
    :size="size"
    @update:value="emitValue"
  />

  <n-input
    v-else-if="definition.type === 'password'"
    :value="value ?? ''"
    type="password"
    show-password-on="click"
    :maxlength="maxLength"
    :placeholder="placeholder"
    :disabled="disabled"
    :size="size"
    @update:value="emitValue"
  />

  <n-input-number
    v-else-if="definition.type === 'int'"
    :value="value ?? null"
    :precision="0"
    :step="1"
    :min="definition.rules.min ?? undefined"
    :max="definition.rules.max ?? undefined"
    :placeholder="placeholder"
    :disabled="disabled"
    :size="size"
    class="w-full"
    @update:value="emitValue"
  />

  <!-- No precision here: fixing it would round 0.5 away. -->
  <n-input-number
    v-else-if="definition.type === 'float'"
    :value="value ?? null"
    :min="definition.rules.min ?? undefined"
    :max="definition.rules.max ?? undefined"
    :placeholder="placeholder"
    :disabled="disabled"
    :size="size"
    class="w-full"
    @update:value="emitValue"
  />

  <GSwitch
    v-else-if="definition.type === 'bool'"
    :value="value === true"
    :disabled="disabled"
    :size="size"
    @update:value="emitValue"
  />

  <n-select
    v-else-if="definition.type === 'select'"
    :value="value === '' ? null : value"
    :options="selectOptions"
    filterable
    :tag="definition.allowCustom"
    :clearable="!definition.rules.required"
    :placeholder="placeholder"
    :disabled="disabled"
    :size="size"
    @update:value="emitSelectValue"
  />

  <!-- Fallback for a type this build does not know: a plain input still lets the
       value be read and saved, which rendering nothing would not. -->
  <n-input
    v-else
    :value="value ?? ''"
    type="text"
    clearable
    :maxlength="maxLength"
    :placeholder="placeholder"
    :disabled="disabled"
    :size="size"
    @update:value="emitValue"
  />
</template>

<script setup>
import { computed } from 'vue'
import { NInput, NInputNumber, NSelect } from 'naive-ui'
import { GSwitch } from '@gameap/ui'
import { getCurrentLanguage } from '@/i18n/i18n'
import { localizedOptionLabel } from '@/parts/gameModVars'

const props = defineProps({
  /** normalizeVarDefinition() output */
  definition: {
    type: Object,
    required: true,
  },
  value: {
    type: [String, Number, Boolean],
    default: null,
  },
  disabled: {
    type: Boolean,
    default: false,
  },
  placeholder: {
    type: String,
    default: '',
  },
  size: {
    type: String,
    default: undefined,
  },
})

const emit = defineEmits(['update:value'])

const maxLength = computed(() => props.definition.rules.maxLength ?? undefined)

const selectOptions = computed(() => {
  const locale = getCurrentLanguage()

  const options = props.definition.options.map((option) => ({
    label: localizedOptionLabel(option, locale),
    value: option.value,
  }))

  // A value that is not in `options` still has to be shown: a custom value saved
  // earlier under allow_custom, or a value left behind when the option list
  // changed. Without this the select renders blank and a save would wipe it;
  // an out-of-list value is reported by the validation rule instead.
  const current = props.value
  if (
    current !== null && current !== undefined && current !== ''
    && !options.some((option) => option.value === current)
  ) {
    options.unshift({ label: String(current), value: String(current) })
  }

  return options
})

function emitValue(value) {
  emit('update:value', value)
}

function emitSelectValue(value) {
  emit('update:value', value ?? '')
}
</script>
