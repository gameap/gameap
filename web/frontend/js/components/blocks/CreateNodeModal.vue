<template>
  <n-modal class="create-node-modal" v-model:show="showModal">
    <template #header-extra>
      <button type="button" class="btn-close" aria-label="Close" v-on:click="showModal = false">
        <i class="fa-solid fa-xmark"></i>
      </button>
    </template>

    <n-card
        :title="trans('dedicated_servers.autosetup_title')"
        style="max-width: 800px;min-height: 500px"
        :bordered="false"
        size="huge"
        role="dialog"
        aria-modal="true"
    >
      <n-tabs type="line" class="flex justify-between" animated>
    <n-tab-pane name="linux">
      <template #tab>
        <i class="fa-brands fa-linux mr-1"></i>Linux
      </template>

      <div class="md:w-full pr-4 pl-4 m-6"
           v-html="trans('dedicated_servers.autosetup_description_linux', {
              'host': host,
              'token': token,
          })
          + '<code class=\'curl-link\'>curl '+link+' | bash --</code>'
          + '<p class=\'text-center\'><small>'+trans('dedicated_servers.autosetup_expire_msg')+'</small></p>'
          "
      >
      </div>
    </n-tab-pane>

    <n-tab-pane name="windows">
      <template #tab>
        <i class="fa-brands fa-windows mr-1"></i>Windows
      </template>

      <div class="md:w-full pr-4 pl-4 m-6"
           v-html="trans('dedicated_servers.autosetup_description_windows', {
              'host': host,
              'token': token,
           })
           + '<p class=\'text-center\'><small>'
              +trans('dedicated_servers.autosetup_expire_token_msg')
              +'</small></p>'"
      >
      </div>
    </n-tab-pane>
  </n-tabs>
    </n-card>
  </n-modal>
</template>

<script setup>
import {trans} from "../../i18n/i18n";
import {onMounted, ref} from "vue";

const props = defineProps({
  link: '',
  host: '',
  token: '',
});

const showModal = ref(false);

onMounted(() => {
  showModal.value = true;
});

</script>

<style>
.create-node-modal {
  code {
    @apply text-rose-800 dark:text-rose-300 text-sm;
    word-wrap: break-word;
  }

  .curl-link {
    @apply block bg-stone-50 dark:bg-stone-600 p-1 my-1 rounded;
    font-family: monospace;
  }

  ul {
    @apply list-disc mb-2;
  }

  li {
    @apply ml-10;
  }

  p {
    @apply my-1;
  }

  a {
    @apply font-medium text-blue-600 dark:text-blue-500 underline hover:no-underline;
  }
}
</style>