<template>
    <div>
      <div class="w-full">
        <div class="coding inverse-toggle px-5 pt-4 shadow-lg text-stone-100 text-sm font-mono subpixel-antialiased
              bg-stone-800 dark:bg-stone-900 pb-6 pt-4 rounded-lg leading-normal overflow-hidden">
          <div class="top mb-2 flex">
            <div class="h-3 w-3 bg-red-500 rounded-full"></div>
            <div class="ml-2 h-3 w-3 bg-orange-300 rounded-full"></div>
            <div class="ml-2 h-3 w-3 bg-green-500 rounded-full"></div>
          </div>
          <div v-if="!serverActive" class="bg-red-500 text-white dark:bg-red-800 dark:text-stone-200 font-bold rounded px-4 py-2 mt-6 mb-3">
            {{ trans('servers.offline_console_msg') }}
          </div>
          <div ref="consoleRef" class="break-all whitespace-pre-wrap mt-4 flex h-[60vh] overflow-y-scroll overscroll-contain">
            {{ output }}
          </div>

          <div v-if="serverActive && sendCommandAvailable" class="mt-4">
            <div class="relative flex items-stretch w-full">
              <div class="w-full">
                <div class="inline">{{ consoleHostname }}:>&nbsp;</div>
                <i v-if="sendCommandLoading" class="fa-solid fa-gear fa-spin"></i>
                <input
                    v-else
                    v-on:keyup.enter="sendCommand"
                    v-model="inputText"
                    type="text"
                    ref="inputRef"
                    class="terminal-input inline md:w-[40vw] lg:w-[50vw]"
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
import {ref, onMounted, onUnmounted} from 'vue';
import axios from '@/config/axios';
import { replace } from 'lodash-es';
import {
  NDivider,
} from "naive-ui"
import {errorNotification} from "@/parts/dialogs";

const props = defineProps({
  serverId: Number,
  consoleHostname: String,
  serverActive: Boolean,
  sendCommandAvailable: Boolean,
});

const consoleRef = ref();
const inputRef = ref();
const output = ref(null);
const inputText = ref(null);
const lock = ref(false);
const sendCommandLoading = ref(false);
const updateConsole = ref(true);
const autoScroll = ref(true);

function scroll() {
  if (autoScroll.value && consoleRef.value) {
    consoleRef.value.scrollTo({top: consoleRef.value.scrollHeight, behavior: 'smooth'});
  }
}

function getConsole() {
  if (!updateConsole.value) {
    return;
  }

  axios.get(`/api/servers/${props.serverId}/console`)
      .then(response => {
        output.value = replace(response.data.console, /(\r\n|\n|\r)/gm, "\n")
        setTimeout(scroll, 1000);
      })
      .catch(error => {
        console.log(error);
        updateConsole.value = false;
      });
}

function sendCommand() {
  if (lock.value) {
    return;
  }

  lock.value = true;
  sendCommandLoading.value = true;
  axios.post(`/api/servers/${props.serverId}/console`, { command: inputText.value })
      .then(response => {
        inputText.value = '';
        lock.value = false;
        updateConsole.value = true;
        setTimeout(getConsole, 1000);
      })
      .catch(error => {
        lock.value = false;
        console.log(error);
        errorNotification(error)
      }).finally(() => {
        sendCommandLoading.value = false;

        setTimeout(() => {
          if (inputRef.value) {
            inputRef.value.select();
          }

        }, 200);
      });
}

let interval;

onMounted(() => {
  getConsole();
  interval = setInterval(getConsole, 10000);
});
onUnmounted(() => {
  updateConsole.value = false;
  clearInterval(interval);
});
</script>