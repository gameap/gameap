<template>
  <div class="block w-full overflow-auto scrolling-touch" :class="$attrs.class">
    <div class="mb-3">
      <GButton color="green" size="small" v-on:click="addItem">
        <span class="fa-solid fa-plus"></span>
      </GButton>
    </div>

    <n-data-table :columns="columns" :data="items" />

    <div class="flex justify-center mt-2">
      <GButton color="green" size="small" v-on:click="addItem">
        <span class="fa-solid fa-plus"></span>&nbsp;{{ trans('main.add') }}
      </GButton>
    </div>
  </div>
</template>

<script setup>
import GButton from "../GButton.vue";
import {
  NDataTable,
  NInput,
  NSwitch,
} from "naive-ui"
import { ref, reactive, computed, defineModel, h } from 'vue';
import {trans} from "@/i18n/i18n";

const props = defineProps({
  labels: Array,
  keys: Array,
  inputTypes: Array,
  name: String,
});

const items = defineModel()

const classes = reactive({
  'text': 'form-control',
  'checkbox': '',
});

// Methods
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

const columns = computed(() => {
  let result = [];

  for (let i = 0; i < props.labels.length; i++) {
    result.push({
      title: props.labels[i],
      key: props.keys[i],
      render(row, index) {
        switch (props.inputTypes[i]) {
          case 'text':
            return h(NInput, {
              value: row[props.keys[i]],
              onUpdateValue(v) {
                items.value[index][props.keys[i]] = v;
              }
            });
          case 'checkbox':
            return h(NSwitch, {
              value: row[props.keys[i]],
              onUpdateValue(v) {
                items.value[index][props.keys[i]] = v;
              }
            });
        }
      }
    });
  }

  result.push({
    title: trans('main.actions'),
    render(row, index) {
      return [
        h(GButton, {
          color: 'red',
          size: 'small',
          text: trans('main.delete'),
          onClick: () => {
            removeItem(index)
          },
        }, [
          h("i", {class: 'fa-solid fa-times mr-0.5'}),
          h("span", {class: 'hidden lg:inline'}, trans('main.delete')),
        ]),
      ]
    },
  })

  return result;
});
</script>
