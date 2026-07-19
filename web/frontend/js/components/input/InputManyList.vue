<template>
  <div class="block w-full overflow-auto" :class="$attrs.class">
    <div class="mb-3">
      <GButton color="green" size="small" v-on:click="addItem">
        <GIcon name="add" />
      </GButton>
    </div>

    <n-table :bordered="false" :single-line="true" :style="{ minWidth: tableMinWidth }">
      <thead>
        <tr>
          <th v-for="label in labels" :key="label">{{ label }}</th>
          <th>{{ trans('main.actions') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="(row, rowIndex) in items" :key="rowIndex">
          <td v-for="(key, colIndex) in keys" :key="key">
            <template v-if="inputTypes[colIndex] === 'text'">
              <n-input-group>
                <n-input v-model:value="items[rowIndex][key]" />
                <n-button type="default" ghost @click="openTextareaModal(rowIndex, key, row[key])">
                  <GIcon name="maximize" />
                </n-button>
              </n-input-group>
            </template>
            <template v-else-if="inputTypes[colIndex] === 'checkbox'">
              <n-switch v-model:value="items[rowIndex][key]" />
            </template>
          </td>
          <td>
            <GButton color="red" size="small" @click="removeItem(rowIndex)">
              <GIcon name="close" class="mr-0.5" />
              <span class="hidden lg:inline">{{ trans('main.delete') }}</span>
            </GButton>
          </td>
        </tr>
      </tbody>
    </n-table>

    <div class="flex justify-center mt-2">
      <GButton color="green" size="small" v-on:click="addItem">
        <GIcon name="add" />&nbsp;{{ trans('main.add') }}
      </GButton>
    </div>

    <n-modal
      v-model:show="showTextareaModal"
      preset="card"
      :title="trans('main.edit')"
      :bordered="false"
      :segmented="{ content: 'soft', footer: 'soft' }"
      style="width: 600px; max-width: 90vw;"
    >
      <n-input
        v-model:value="textareaValue"
        type="textarea"
        :autosize="{ minRows: 6, maxRows: 20 }"
      />
      <template #footer>
        <div class="flex justify-end gap-2">
          <GButton color="black" @click="closeTextareaModal">
            <GIcon name="close" class="mr-1" />
            {{ trans('main.close') }}
          </GButton>
          <GButton color="green" @click="saveTextareaModal">
            <GIcon name="save" class="mr-1" />
            {{ trans('main.save') }}
          </GButton>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<script setup>
import GButton from "../GButton.vue";
import { GIcon } from '@gameap/ui';
import {
  NInput,
  NSwitch,
  NInputGroup,
  NButton,
  NModal,
  NTable,
} from "naive-ui"
import { ref, computed, defineModel } from 'vue';
import {trans} from "@/i18n/i18n";

const props = defineProps({
  labels: Array,
  keys: Array,
  inputTypes: Array,
  name: String,
});

// An auto-layout table lets n-input (min-width: 0) collapse to a few px next
// to the fixed maximize button; a per-column floor keeps inputs usable and
// hands the overflow to the scrolling wrapper instead.
const tableMinWidth = computed(() => `${props.keys.length * 170 + 110}px`);

const items = defineModel()

const showTextareaModal = ref(false)
const editingRowIndex = ref(null)
const editingKey = ref(null)
const textareaValue = ref('')

const removeItem = (index) => {
  items.value.splice(index, 1);
};

const addItem = () => {
  let emptyItem = {};
  props.keys.forEach((item) => {
    emptyItem[item] = '';
  });

  items.value.push(emptyItem);
};

const openTextareaModal = (index, key, currentValue) => {
  editingRowIndex.value = index
  editingKey.value = key
  textareaValue.value = currentValue || ''
  showTextareaModal.value = true
}

const saveTextareaModal = () => {
  if (editingRowIndex.value !== null && editingKey.value !== null) {
    items.value[editingRowIndex.value][editingKey.value] = textareaValue.value
  }
  closeTextareaModal()
}

const closeTextareaModal = () => {
  showTextareaModal.value = false
  editingRowIndex.value = null
  editingKey.value = null
  textareaValue.value = ''
}
</script>
