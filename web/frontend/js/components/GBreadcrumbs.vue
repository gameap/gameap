<template>
  <!-- Breadcrumb -->
  <nav class="flex pt-3 pb-3 py-4 px-4 mb-4 text-stone-700 border border-stone-200 rounded-lg bg-stone-100 dark:bg-stone-800 dark:border-stone-700" aria-label="Breadcrumb">
    <ol class="inline-flex items-center space-x-1 md:space-x-2 rtl:space-x-reverse">
      <li v-for="item in items.slice(0, 1)" class="inline-flex items-center">
        <a v-if="item.link" :href="item.link" class="inline-flex items-center text-sm font-medium text-stone-700 hover:text-blue-600 dark:text-stone-400 dark:hover:text-white">
          <i v-if="item.icon" :class="item.icon" class="align-middle mr-0.5"></i>
          <span class="align-middle">{{ item.text }}</span>
        </a>
        <router-link v-else-if="item.route" :to="item.route" class="ms-1 text-sm font-medium text-stone-700 hover:text-blue-600 md:ms-2 dark:text-stone-400 dark:hover:text-white">
          <i v-if="item.icon" :class="item.icon" class="align-middle mr-0.5"></i>
          <span class="align-middle">{{ item.text }}</span>
        </router-link>
        <span v-else class="ms-1 text-sm font-medium text-stone-500 md:ms-2 dark:text-stone-400">{{ item.text }}</span>
      </li>

      <li v-for="item in items.slice(1)">
        <div class="flex items-center">
          <i class="fa-solid fa-chevron-right align-middle w-3 h-3 mx-2 text-stone-400"></i>
          <a v-if="item.link" :href="item.link" class="ms-1 text-sm font-medium text-stone-700 hover:text-blue-600 md:ms-2 dark:text-stone-400 dark:hover:text-white">
            <i v-if="item.icon !== ''" :class="item.icon" class="align-middle mr-0.5"></i>
            <span class="align-middle">{{ item.text }}</span>
          </a>
          <router-link v-else-if="item.route" :to="item.route" class="ms-1 text-sm font-medium text-stone-700 hover:text-blue-600 md:ms-2 dark:text-stone-400 dark:hover:text-white">
            <i v-if="item.icon !== ''" :class="item.icon" class="align-middle mr-0.5"></i>
            <span class="align-middle">{{ item.text }}</span>
          </router-link>
          <span v-else-if="item.render" class="ms-1 text-sm font-medium text-stone-500 md:ms-2 dark:text-stone-400">
            <render :render="item.render" />
          </span>
          <span v-else class="ms-1 text-sm font-medium text-stone-500 md:ms-2 dark:text-stone-400">
            {{ item.text }}
          </span>
        </div>
      </li>
    </ol>
  </nav>
</template>

<script setup>
const props = defineProps({
  items: null,
});

const render = (attr) => {
  return attr.render()
}

</script>