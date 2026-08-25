<template>
    <div class="fm-toolbar-group" role="group">
        <button
            type="button"
            class="fm-tool-btn"
            v-bind:class="{ 'fm-tool-btn--toggled': fm.searchOpen }"
            v-bind:title="`${lang.btn.search} (${lang.hint.ctrlF})`"
            v-on:click="fm.toggleSearch()"
        >
            <GIcon name="search" />
        </button>
        <template v-if="fm.searchOpen">
            <input
                ref="inputEl"
                class="fm-search-input"
                type="text"
                autocomplete="off"
                spellcheck="false"
                v-bind:value="fm.searchQuery"
                v-bind:placeholder="lang.manager.searchPlaceholder"
                v-on:input="fm.setSearchQuery($event.target.value)"
                v-on:keydown="onKeydown"
            />
            <button
                type="button"
                class="fm-tool-btn"
                v-bind:title="`${lang.btn.searchPrev} (${lang.hint.shiftEnter})`"
                v-on:mousedown.prevent
                v-on:click="fm.searchPrev()"
            >
                <GIcon name="chevron-up" />
            </button>
            <button
                type="button"
                class="fm-tool-btn"
                v-bind:title="`${lang.btn.searchNext} (${lang.hint.enter})`"
                v-on:mousedown.prevent
                v-on:click="fm.searchNext()"
            >
                <GIcon name="chevron-down" />
            </button>
            <button
                type="button"
                class="fm-tool-btn"
                v-bind:title="`${lang.btn.searchClose} (${lang.hint.esc})`"
                v-on:mousedown.prevent
                v-on:click="fm.closeSearch()"
            >
                <GIcon name="close" />
            </button>
        </template>
    </div>
</template>

<script setup>
import { ref, watch, nextTick } from 'vue'
import { GIcon } from '@gameap/ui'
import { useFileManagerStore } from '../../stores/useFileManagerStore.js'
import { useTranslate } from '../../composables/useTranslate.js'

const fm = useFileManagerStore()
const { lang } = useTranslate()

const inputEl = ref(null)

// Single focus mechanism for every open path: 🔎 click, Ctrl+F, type-ahead.
// nextTick because on open the input is still behind v-if.
watch(
    () => fm.searchFocusRequestId,
    () => {
        nextTick(() => {
            const el = inputEl.value
            if (!el) return

            el.focus()
            const end = el.value.length
            el.setSelectionRange(end, end)
        })
    }
)

function onKeydown(event) {
    switch (event.key) {
        case 'Enter':
            event.preventDefault()
            if (event.shiftKey) {
                fm.searchPrev()
            } else {
                fm.searchNext()
            }
            break
        case 'ArrowDown':
            event.preventDefault()
            fm.searchNext()
            break
        case 'ArrowUp':
            event.preventDefault()
            fm.searchPrev()
            break
        case 'Escape':
            event.preventDefault()
            event.stopPropagation()
            fm.closeSearch()
            break
        default:
            break
    }
}
</script>

<style lang="scss">
.fm-search-input {
    @apply w-44 sm:w-56 bg-transparent px-2.5 text-sm text-body placeholder:text-faint border-r;
    border-top: none;
    border-bottom: none;
    border-left: none;
    outline: none;
    min-width: 0;
    height: var(--fm-control-height);
}
</style>
