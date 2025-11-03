<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <section>
    <div class="py-8 px-4 mx-auto max-w-screen-xl lg:py-16 lg:px-6">
      <div class="mx-auto max-w-screen-sm text-center">
        <h1 class="mb-4 text-7xl tracking-tight font-extrabold lg:text-9xl text-stone-600 dark:text-stone-500">403</h1>
        <p class="mb-4 text-3xl tracking-tight font-bold text-stone-900 md:text-4xl dark:text-white">{{ trans('main.something_went_wrong')}}</p>
        <GButton size="large" color="black" :route="{name: 'home'}">
          {{ trans('navbar.main') }}
        </GButton>
      </div>
    </div>
  </section>
</template>

<script setup>
import GBreadcrumbs from "../../components/GBreadcrumbs.vue";
import {trans} from "../../i18n/i18n";
import GButton from "../../components/GButton.vue";

const breadcrumbs = computed(() => {
  return [
    {route:'/', text:'GameAP', icon: 'gicon gicon-gameap'},
    {text: 404},
  ]
})
</script>