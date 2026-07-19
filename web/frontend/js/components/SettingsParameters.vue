<template>
    <div>
        <div class="flex flex-col min-w-0 rounded break-words border bg-white border-1 border-stone-300 dark:bg-stone-900 dark:border-stone-700 mb-2">
            <div class="flex-auto p-6">
                <div v-for="(item, itemIndex) in items">
                    <div class="mb-3">
                        <label v-bind:for="inputName + '[' + itemIndex + ']' + '[name]'" class="control-label">{{ trans('labels.name') }}</label>
                        <input
                                v-bind:name="inputName + '[' + itemIndex + ']' + '[name]'"
                                v-bind:id="inputName + '[' + itemIndex + ']' + '[name]'"
                                v-model="item.name"
                                type="text"
                                class="form-control">
                    </div>

                    <div class="mb-3">
                        <label v-bind:for="inputName + '[' + itemIndex + ']' + '[value]'" class="control-label">{{ trans('labels.the_value') }}</label>
                        <textarea
                                v-bind:name="inputName + '[' + itemIndex + ']' + '[value]'"
                                v-bind:id="inputName + '[' + itemIndex + ']' + '[value]'"
                                v-model="item.value"
                                class="form-control"
                                v-bind:rows='(item.value.match(new RegExp("\n", "g")) || []).length + 2'>
                        </textarea>
                    </div>

                    <button class="inline-block align-middle text-center select-none border font-normal whitespace-nowrap rounded  no-underline py-1.5 px-2 leading-tight text-xs  bg-red-600 text-white hover:bg-red-700" v-on:click.prevent="removeItem(itemIndex)">
                        <GIcon name="close" /> {{ trans('main.delete') }}
                    </button>

                    <hr>
                </div>
                <div class="mt-2 text-center">
                  <button class="inline-block align-middle text-center select-none border font-normal whitespace-nowrap rounded  no-underline py-1.5 px-2 leading-tight text-xs  bg-lime-500 text-white hover:bg-lime-600" v-on:click.prevent="addItem"><GIcon name="add" />&nbsp;{{ trans('main.add') }}</button>
                </div>
            </div>
        </div>
    </div>
</template>

<script>
    import { GIcon } from '@gameap/ui'

    export default {
        name: "SettingsParameters",
        components: { GIcon },
        props: {
            initialItems: Array,
            inputName: String,
        },
        data: function () {
            return {
                items: this.initialItems || [],
            }
        },
        methods: {
            addItem: function() {
                this.items.push({
                    name: '',
                    value: ''
                });
            },
            removeItem: function(index) {
                this.items.splice(index, 1);
            }
        }
    }
</script>
