<template>
    <div class="modal-content fm-modal-properties">
        <div class="modal-header grid grid-cols-2">
            <h5 class="modal-title">{{ lang.modal.properties.title }}</h5>
            <button type="button" class="btn-close" aria-label="Close" v-on:click="hideModal">
                <i class="fa-solid fa-xmark"></i>
            </button>
        </div>
        <div class="modal-body">
            <div class="grid grid-cols-3 gap-4 my-3">
                <div><strong>{{ lang.modal.properties.disk }}:</strong></div>
                <div>{{ selectedDisk }}</div>
                <div class="text-right">
                    <i
                        v-on:click="copyToClipboard(selectedDisk)"
                        v-bind:title="lang.clipboard.copy"
                        class="fa-regular fa-copy"
                    />
                </div>
            </div>
            <div class="grid grid-cols-3 gap-4 my-3">
                <div><strong>{{ lang.modal.properties.name }}:</strong></div>
                <div>{{ selectedItem.basename }}</div>
                <div class="text-right">
                    <i
                        v-on:click="copyToClipboard(selectedItem.basename)"
                        v-bind:title="lang.clipboard.copy"
                        class="fa-regular fa-copy"
                    />
                </div>
            </div>
            <div class="grid grid-cols-3 gap-4 my-3">
                <div><strong>{{ lang.modal.properties.path }}:</strong></div>
                <div>{{ selectedItem.path }}</div>
                <div class="text-right">
                    <i
                        v-on:click="copyToClipboard(selectedItem.path)"
                        v-bind:title="lang.clipboard.copy"
                        class="fa-regular fa-copy"
                    />
                </div>
            </div>
            <template v-if="selectedItem.type === 'file'">
                <div class="grid grid-cols-3 gap-4 my-3">
                    <div><strong>{{ lang.modal.properties.size }}:</strong></div>
                    <div>{{ bytesToHuman(selectedItem.size) }}</div>
                    <div class="text-right">
                        <i
                            v-on:click="copyToClipboard(bytesToHuman(selectedItem.size))"
                            v-bind:title="lang.clipboard.copy"
                            class="fa-regular fa-copy"
                        />
                    </div>
                </div>
            </template>
            <template v-if="selectedItem.hasOwnProperty('timestamp')">
                <div class="grid grid-cols-3 gap-4 my-3">
                    <div><strong>{{ lang.modal.properties.modified }}:</strong></div>
                    <div>{{ timestampToDate(selectedItem.timestamp) }}</div>
                    <div class="text-right">
                        <i
                            v-on:click="copyToClipboard(timestampToDate(selectedItem.timestamp))"
                            v-bind:title="lang.clipboard.copy"
                            class="fa-regular fa-copy"
                        />
                    </div>
                </div>
            </template>
            <template v-if="selectedItem.hasOwnProperty('acl')">
                <div class="grid grid-cols-3 gap-4 my-3">
                    <div>{{ lang.modal.properties.access }}:</div>
                    <div>{{ lang.modal.properties['access_' + selectedItem.acl] }}</div>
                </div>
            </template>
        </div>
    </div>
</template>

<script>
import modal from '../mixins/modal.js';
import translate from '../../../mixins/translate.js';
import helper from '../../../mixins/helper.js';
import EventBus from '../../../emitter.js';

export default {
    name: 'PropertiesModal',
    mixins: [modal, translate, helper],
    data() {
        return {
            url: null,
        };
    },
    computed: {
        /**
         * Selected disk
         * @returns {*}
         */
        selectedDisk() {
            return this.$store.getters['fm/selectedDisk'];
        },

        /**
         * Selected file
         * @returns {*}
         */
        selectedItem() {
            return this.$store.getters['fm/selectedItems'][0];
        },
    },
    methods: {
        /**
         * Get URL
         */
        getUrl() {
            this.$store
                .dispatch('fm/url', {
                    disk: this.selectedDisk,
                    path: this.selectedItem.path,
                })
                .then((response) => {
                    if (response.data.result.status === 'success') {
                        this.url = response.data.url;
                    }
                });
        },

        /**
         * Copy text to clipboard
         * @param text
         */
        copyToClipboard(text) {
            // create input
            const copyInputHelper = document.createElement('input');
            copyInputHelper.className = 'copyInputHelper';
            document.body.appendChild(copyInputHelper);
            // add text
            copyInputHelper.value = text;
            copyInputHelper.select();
            // copy text to clipboard
            document.execCommand('copy');
            // clear
            document.body.removeChild(copyInputHelper);

            // Notification
            EventBus.emit('addNotification', {
                status: 'success',
                message: this.lang.notifications.copyToClipboard,
            });
        },
    },
};
</script>

<style lang="scss">
.fm-modal-properties .modal-body {
    .row {
        margin-bottom: 0.3rem;
        padding-top: 0.3rem;
        padding-bottom: 0.3rem;

        &:hover {
            background-color: #f8f9fa;
        }
    }
}
</style>
