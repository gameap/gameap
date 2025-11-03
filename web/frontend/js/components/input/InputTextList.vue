<template>
    <div>
        <div class="mb-3 deletable" v-for="(item, index) in items" :key="index">
            <div class="flex flex-wrap ">
                <div class="md:w-full pr-4 pl-4 relative flex items-stretch w-full">
                  <n-input type="text" v-model:value="items[index]"></n-input>

                  <GButton color="red" size="small" class="ml-1" v-on:click="removeItem(index)">
                    <span class="fa-solid fa-times"></span>
                  </GButton>
                </div>
            </div>
        </div>

        <div class="flex justify-center mt-2">
          <GButton color="green" size="small" v-on:click="addItem">
            <span class="fa-solid fa-plus"></span>
          </GButton>
        </div>
    </div>
</template>

<script setup>
import { defineProps, defineModel } from 'vue'
import {trans} from "../../i18n/i18n"
import GButton from "../GButton.vue"
import {
  NDataTable,
  NInput,
  NSwitch,
} from "naive-ui"

const props = defineProps({
  label: String,
  name: String
});

const items = defineModel()

const removeItem = (index) => {
  items.value.splice(index, 1);
};

const addItem = () => {
  if (!items.value) {
    items.value = ['']
  } else {
    items.value.push('');
  }
};
</script>
