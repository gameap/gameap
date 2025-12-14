<template>
    <div class="p-4">
        <h1 class="text-xl font-bold mb-4 text-stone-900 dark:text-white">
            {{ trans('title') }}
        </h1>
        <p class="text-stone-600 dark:text-stone-400 mb-4">
            {{ trans('welcome') }}
        </p>

        <div class="p-4 border border-stone-200 dark:border-stone-700 rounded-lg bg-white dark:bg-stone-800">
            <h2 class="font-semibold mb-2 text-stone-900 dark:text-white">
                {{ trans('user_info') }}
            </h2>
            <p class="text-stone-600 dark:text-stone-400">
                {{ trans('logged_in_as') }}: <strong>{{ user.login }}</strong>
            </p>
            <p class="text-stone-600 dark:text-stone-400">
                {{ trans('admin') }}: {{ user.isAdmin ? trans('yes') : trans('no') }}
            </p>
        </div>
    </div>
</template>

<script setup lang="ts">
import { useCurrentUser, usePluginTrans } from '@gameap/plugin-sdk';

const user = useCurrentUser();
const { trans } = usePluginTrans();
</script>
