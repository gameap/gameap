<script setup>
  import { ref } from "vue";

  const props = defineProps({
      options: null,
      name: "",
      value: null,
  });

  const name = ref(props.name);
  const value = ref(props.value);
</script>


<template>
    <n-select v-model:value="value" multiple :options="options" />

    <div v-if="value !== undefined && value !== null" v-for="item in value">
        <input type="hidden" :name="name" :value="item" />
    </div>
</template>
