<template>
    <div class="w-full mt-5 p-3 border border-stone-200 bg-stone-50 rounded-lg sm:p-4 dark:bg-stone-800 dark:border-stone-700">
        <strong class="text-stone-900 dark:text-white">
            <i class="fas fa-puzzle-piece mr-1"></i>
            My Plugin Widget
        </strong>
        <p class="mt-2 text-stone-600 dark:text-stone-400">
            This widget was added by your plugin. It appears on the dashboard.
        </p>
        <p v-if="isAdmin" class="mt-2 text-sm text-green-600 dark:text-green-400">
            You are viewing this as an admin.
        </p>
    </div>
</template>

<script setup lang="ts">
import type { DashboardWidgetProps } from '@gameap/plugin-sdk';

const props = defineProps<DashboardWidgetProps>();

const isAdmin = props.isAdmin;
</script>
