<template>
  <div :class="$attrs.class">
    <div v-for="item in items" class="flex mb-1 md:w-full">
      <div
          class="text-xs text-center h-full md:w-full inline-flex px-2 py-1 bg-stone-500 hover:bg-stone-600 text-white rounded-s cursor-pointer"
          @click="onClick(item.gameCode, item.id)"
      >
        {{ item.name }}
      </div>
      <div
          class="ml-[2px] text-xs h-full inline-flex items-center px-2 py-1 bg-red-500 hover:bg-red-600 text-white rounded-e cursor-pointer"
          @click="onClickDelete(item.id)"
      >
        <i class="fa-solid fa-trash"></i>&nbsp;
      </div>
    </div>
  </div>

</template>

<script setup>
const props = defineProps({
  items: [],
  clickCallback: Function,
  deleteCallback: Function,
});

const onClickDelete = (id) => {
  props.deleteCallback(id)
}

const onClick = (code, id) => {
  props.clickCallback(code, id)
}

</script>