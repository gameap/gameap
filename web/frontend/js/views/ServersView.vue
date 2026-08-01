<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <ServerMainList />
</template>

<script setup>
import { GBreadcrumbs } from "@gameap/ui";
import {computed, defineAsyncComponent} from "vue";
import ServerMainList from "@/views/servertabs/ServerMainList.vue";
import {trans} from "@/i18n/i18n";

const breadcrumbs = computed(() => {
  return [
    {'route':'/', 'text':'GameAP', 'icon': 'gameap'},
    {'route':{name: 'servers'}, 'text':trans('servers.game_servers')},
  ]
})
</script>