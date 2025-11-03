<template>
  <table class="stone-table">
    <thead v-if="titleKey || titleValue " class="stone-table-header">
      <tr>
        <th class="px-2 py-2">{{ titleKey }}</th>
        <th class="px-2 py-2">{{ titleValue }}</th>
      </tr>
    </thead>
    <tbody>
      <tr v-for="(value, key) in items" class="stone-table-row">
        <th class="px-2 py-2">{{ key }}</th>
        <td class="px-2 py-2">{{ value }}</td>
      </tr>
    </tbody>
  </table>
</template>


<script setup>
const props = defineProps({
  titleKey: '',
  titleValue: '',
  items: null,
});
</script>