<template>
    <div class="modal-content fm-modal-text-edit">
        <div class="modal-header grid grid-cols-2">
            <h5 class="modal-title w-75 text-truncate">
                {{ lang.modal.editor.title }}
                <small class="text-muted pl-3">{{ selectedItem.basename }}</small>
            </h5>
            <button type="button" class="btn-close" aria-label="Close" v-on:click="hideModal">
                <i class="fa-solid fa-xmark"></i>
            </button>
        </div>
        <div class="modal-body">
            <div v-if="codeLoaded">
                <codemirror
                    ref="fmCodeEditor"
                    v-model="code"
                    :style="{ height: editorHeight + 'px' }"
                    :extensions="extensions"
                    v-on:change="onChange"
                />
            </div>
            <div class="p-5" v-else :style="{ height: editorHeight + 'px' }">
                <div class="d-flex justify-content-center">
                    <div class="spinner-border spinner-border-big" role="status"></div>
                </div>
            </div>
        </div>
        <div class="modal-footer mt-2">
            <button type="button" class="btn btn-info mr-2 rounded" v-on:click="updateFile">
                {{ lang.btn.submit }}
            </button>
            <button type="button" class="btn btn-light rounded" v-on:click="hideModal">
                {{ lang.btn.cancel }}
            </button>
        </div>
    </div>
</template>

<script>
import { Codemirror } from 'vue-codemirror'
import { javascript } from '@codemirror/lang-javascript'
import { xml } from '@codemirror/lang-xml'
import { json } from "@codemirror/lang-json";
import { oneDark } from '@codemirror/theme-one-dark'

import modal from '../mixins/modal';
import translate from '../../../mixins/translate';

export default {
    name: 'TextEditModal',
    mixins: [modal, translate],
    components: { Codemirror },
    data() {
        return {
            code: '',
            extensions: [javascript(), xml(), json(), oneDark],
            editedCode: '',
            codeLoaded: false,
        };
    },
    mounted() {
        // get file for edit
        this.$store
            .dispatch('fm/getFile', {
                disk: this.selectedDisk,
                path: this.selectedItem.path,
            })
            .then((response) => {
                // add code
                if (this.selectedItem.extension === 'json') {
                    this.code = JSON.stringify(response.data, null, 4);
                } else {
                    this.code = response.data;
                }

                this.codeLoaded = true;
            })
            .catch(() => {
                this.hideModal();
            });
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

        /**
         * CodeMirror options
         * @returns {{mode: *, lineNumbers: boolean, line: boolean, theme: string}}
         */
        cmOptions() {
            return {
                mode: this.$store.state.fm.settings.textExtensions[this.selectedItem.extension],
                theme: 'blackboard',
                lineNumbers: true,
                line: true,
            };
        },

        /**
         * Calculate the height of the code editor
         * @returns {number}
         */
        editorHeight() {
            if (this.$store.state.fm.modal.modalBlockHeight) {
                return this.$store.state.fm.modal.modalBlockHeight - 200;
            }

            return 300;
        },
    },
    methods: {
        /**
         * Update file
         */
        updateFile() {
            const formData = new FormData();
            // add disk name
            formData.append('disk', this.selectedDisk);
            // add path
            formData.append('path', this.selectedItem.dirname);
            // add updated file
            formData.append('file', new Blob([this.editedCode]), this.selectedItem.basename);

            this.$store.dispatch('fm/updateFile', formData).then((response) => {
                if (response.data.result.status === 'success') {
                    this.hideModal();
                }
            });
        },

        /**
         * Edited code
         * @param value
         */
        onChange(value) {
            this.editedCode = value;
        },
    },
};
</script>

<style lang="scss">

.fm-modal-text-edit {
    .modal-body {
        padding: 0;
    }
}

.spinner-border-big {
    width: 3rem;
    height: 3rem;
}
</style>
