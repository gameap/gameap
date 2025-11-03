<template>
  <section>
    <div class="py-8 px-4 mx-auto max-w-screen-xl lg:py-16 lg:px-6">
      <div class="mx-auto max-w-screen-sm text-center">
        <h2 class="mb-4 text-4xl tracking-tight font-extrabold lg:text-5xl text-stone-600 dark:text-stone-500"><i class="fa-solid fa-skull"></i></h2>
        <h1 class="mb-4 text-7xl tracking-tight font-extrabold lg:text-9xl text-stone-600 dark:text-stone-500">500</h1>
        <p class="mb-4 text-3xl tracking-tight font-bold text-stone-900 md:text-4xl dark:text-white">{{ trans('main.something_went_wrong')}}</p>
      </div>
    </div>
  </section>
</template>

<script setup>
import {trans} from "@/i18n/i18n";
</script>