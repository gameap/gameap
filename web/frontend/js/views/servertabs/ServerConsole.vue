<template>
    <div>
      <div class="w-full">
        <div class="coding inverse-toggle px-5 pt-4 shadow-lg text-terminal text-sm font-mono subpixel-antialiased
              bg-terminal pb-6 rounded-lg leading-normal overflow-hidden">
          <div class="top mb-2 flex">
            <div class="h-3 w-3 bg-red-500 rounded-full"></div>
            <div class="ml-2 h-3 w-3 bg-orange-300 rounded-full"></div>
            <div class="ml-2 h-3 w-3 bg-lime-500 rounded-full"></div>
          </div>
          <div v-if="!serverActive" class="bg-red-500 text-white dark:bg-red-800 dark:text-stone-200 font-bold rounded px-4 py-2 mt-6 mb-3">
            {{ trans('servers.offline_console_msg') }}
          </div>
          <div v-if="serverActive && closeReason" class="bg-warning text-white dark:bg-orange-700 dark:text-stone-200 font-bold rounded px-4 py-2 mt-6 mb-3">
            {{ trans('servers.console_unavailable') }}: {{ closeReason }}
          </div>
          <div ref="consoleRef" class="break-all whitespace-pre-wrap mt-4 flex h-[60vh] overflow-y-scroll overscroll-contain">
            {{ output }}
          </div>

          <div v-if="serverActive && sendCommandAvailable" class="mt-4">
            <div class="relative flex items-stretch w-full">
              <div class="w-full">
                <div class="inline">{{ consoleHostname }}:>&nbsp;</div>
                <input
                    v-on:keyup.enter="sendCommand"
                    v-model="inputText"
                    type="text"
                    ref="inputRef"
                    class="terminal-input inline md:w-[40vw] lg:w-[50vw]"
                    :class="{ 'opacity-50 cursor-not-allowed': closeReason }"
                    :disabled="!!closeReason"
                    :placeholder="trans('servers.enter_command') +' ...'"
                >
              </div>
            </div>
          </div>
          <NDivider dashed></NDivider>
          <div class="p-1 cursor-pointer inline" @click="autoScroll = !autoScroll">
            <span v-if="autoScroll">[x]</span>
            <span v-else>[&nbsp;]</span>
            {{ trans('main.autoscroll')}}
          </div>
        </div>
      </div>
    </div>
</template>

<script setup>
import {ref, computed, watch, nextTick} from 'vue';
import { replace } from 'lodash-es';
import {
  NDivider,
} from "naive-ui"
import { useAttachWebSocket } from '@/composables/useAttachWebSocket'

const props = defineProps({
  serverId: Number,
  consoleHostname: String,
  serverActive: Boolean,
  sendCommandAvailable: Boolean,
});

const consoleRef = ref();
const inputRef = ref();
const inputText = ref('');
const autoScroll = ref(true);

const { output: rawOutput, sendInput, closeReason, attached } = useAttachWebSocket(props.serverId)

const output = computed(() => {
  if (!rawOutput.value) return ''
  return replace(rawOutput.value, /(\r\n|\n|\r)/gm, '\n')
})

function scroll() {
  if (autoScroll.value && consoleRef.value) {
    consoleRef.value.scrollTo({top: consoleRef.value.scrollHeight, behavior: 'smooth'});
  }
}

watch(output, () => {
  nextTick(scroll)
})

function sendCommand() {
  const command = inputText.value.trim()
  if (!command) return
  sendInput(command + '\n')
  inputText.value = ''
  nextTick(() => {
    if (inputRef.value) {
      inputRef.value.select()
    }
  })
}
</script>
