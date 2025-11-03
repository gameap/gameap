<template>
    <div id="rcon-console-component">
      <div class="coding inverse-toggle px-5 pt-4 shadow-lg text-stone-100 text-sm font-mono subpixel-antialiased
              bg-stone-800 dark:bg-stone-900 pb-6 pt-4 rounded-lg leading-normal overflow-hidden
      ">
        <div ref="consoleRef" class="whitespace-pre-wrap mt-4 flex h-[40vh] overflow-y-scroll overscroll-contain">
          <div v-if="loading" class="flex w-full items-center justify-center">
            <Loading></Loading>
          </div>
          <div v-else>
            {{ output }}
          </div>
        </div>
      </div>

      <div v-if="fastRcon" class="gap-x-2 mt-2">
        <span
            v-for="fastCommand in fastRcon"
            v-on:click="setAndSendCommand(fastCommand.command)"
            class="bg-stone-100 hover:bg-stone-200 text-stone-800 text-xs font-medium me-2
            px-2.5 py-1 rounded dark:bg-stone-700 dark:text-stone-300 cursor-pointer">
          {{ fastCommand.info }}
        </span>
      </div>

      <div class="grid grid-cols-8 gap-x-2 mt-2">
        <div class="col-span-7 w-full">
          <NInput
              v-model:value="command"
              v-on:keyup.enter="sendCommand"
              :disabled="loading"
              type="text"
              placeholder=""
          />
        </div>

        <GButton color="black" size="small" v-on:click="sendCommand">
          <i class="fa-solid fa-terminal"></i>
          <span class="hidden lg:inline">&nbsp;{{ trans('main.send') }}</span>
        </GButton>

      </div>
    </div>
</template>

<script setup>
import {computed, ref, onMounted, defineProps} from "vue"
import {
  NInput,
} from "naive-ui"
import {storeToRefs} from "pinia"
import GButton from "../GButton.vue"
import {errorNotification} from "../../parts/dialogs"
import Loading from "../Loading.vue"
import {useServerRconStore} from "../../store/serverRcon";

const serverRconStore = useServerRconStore()

const {output, fastRcon} = storeToRefs(serverRconStore)

const props = defineProps({
  serverId: null
})

const command = ref('')
const loading = computed(() => serverRconStore.loading)

const sendCommand = () => {
  serverRconStore.sendCommand(command.value).
  then(() => {
    command.value = ''
  }).
  catch((error) => {
    errorNotification(error)
  })
}

const setAndSendCommand = (fastCommand) => {
  command.value = fastCommand
  sendCommand()
}

onMounted(() => {
  serverRconStore.fetchFastRcon()
})
</script>