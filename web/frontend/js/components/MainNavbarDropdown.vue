<template>
  <GMenu as="div" :class="$attrs.class" class="relative inline-flex items-center text-left">
    <div class="sm:hidden">
      <GMenuButton @click="onMenuButtonClick" class="flex items-center gap-x-1.5 text-white hover:bg-chrome-item px-4 py-2 rounded">
        <GIcon v-if="buttonIcon" :name="buttonIcon" />
      </GMenuButton>
    </div>

    <div class="hidden sm:block">
      <GMenuButton @click="onMenuButtonClick" class="flex items-center w-full gap-x-1.5 text-white hover:bg-chrome-item md:px-4 md:py-2 rounded">
        <GIcon v-if="buttonIcon" :name="buttonIcon" class="mr-1" />
        <span class="md:visible collapse">{{ props.buttonText }}</span>
        <GIcon v-if="!menuOpened" name="chevron-down" class="ml-4 md:visible collapse" />
        <GIcon v-else name="chevron-up" class="ml-4 md:visible collapse" />
      </GMenuButton>
    </div>

    <transition @after-leave="transitionAfterLeave" enter-active-class="transition ease-out duration-100" enter-from-class="transform opacity-0 scale-95" enter-to-class="transform opacity-100 scale-100" leave-active-class="transition ease-in duration-75" leave-from-class="transform opacity-100 scale-100" leave-to-class="transform opacity-0 scale-95">
      <GMenuItems :unmount="menuItemsUnmount" class="absolute right-0 top-full z-10 mt-2 w-56 origin-top-right divide-y divide-stone-100 rounded bg-white dark:bg-stone-900 shadow-lg ring-1 ring-black ring-opacity-5 focus:outline-none">
        <div class="py-1" v-for="itemGroup in items">
          <GMenuItem v-slot="{ active, close }" v-for="item in itemGroup">
            <a
                v-if="item.route"
                @click="onLinkClick(item, close)"
                :class="[active
                  ? 'bg-stone-100 text-stone-900 dark:bg-stone-950 dark:text-stone-300'
                  : 'text-stone-700 dark:text-stone-200',
                  'block px-4 py-2 text-sm cursor-pointer'
                ]"
            >
                <GIcon v-if="item.icon" :name="item.icon" />
                {{ item.label }}
            </a>
            <a
                v-else-if="item.link"
                :href="item.link"
                @click="onMenuButtonClick"
                :class="[active
                  ? 'bg-stone-100 text-stone-900 dark:bg-stone-950 dark:text-stone-300'
                  : 'text-stone-700 dark:text-stone-200',
                  'block px-4 py-2 text-sm'
                ]"
            >
              <GIcon v-if="item.icon" :name="item.icon" />
              {{ item.label }}
            </a>
            <a
                v-else-if="item.onClick"
                :href="item.link"
                @click="item.onClick"
                :class="[active
                  ? 'bg-stone-100 text-stone-900 dark:bg-stone-950 dark:text-stone-300'
                  : 'text-stone-700 dark:text-stone-200',
                  'block px-4 py-2 text-sm cursor-pointer'
                ]"
            >
              <GIcon v-if="item.icon" :name="item.icon" />
              {{ item.label }}
            </a>
          </GMenuItem>
        </div>
      </GMenuItems>
    </transition>
  </GMenu>
</template>

<script setup>
import { GMenu, GMenuButton, GMenuItem, GMenuItems, GIcon } from "@gameap/ui"
import {defineProps, ref} from "vue"
import {useRouter} from "vue-router"

const router = useRouter()

const props = defineProps({
  buttonText: {
    type: String,
    required: true,
  },
  buttonIcon: {
    type: String,
    required: false,
  },
  items: {
    type: Array,
    required: false,
  },
})

const menuItemsUnmount = ref(true)
const menuOpened = ref(false)

const onMenuButtonClick = () => {
  menuOpened.value = !menuOpened.value;
}

const onLinkClick = (item, close) => {
  menuOpened.value = false
  close()

  if (item.route) {
    router.push(item.route)
  }
};

const transitionAfterLeave = () => {
  menuOpened.value = false
}


</script>