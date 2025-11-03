<template>
    <div id="server-task-component">
        <div class="mb-2">
          <GButton color="green" size="small" v-on:click="createTask()">
            <i class="fa fa-plus-square"></i>
            {{ trans('main.add') }}
          </GButton>
        </div>

      <table class="stone-table">
        <thead class="stone-table-header">
        <tr>
          <th scope="col" class="px-4 py-4">{{ trans('servers_tasks.task') }}</th>
          <th scope="col" class="px-4 py-4">{{ trans('servers_tasks.date') }}</th>
          <th scope="col" class="px-4 py-4">{{ trans('servers_tasks.repeat') }}</th>
          <th scope="col" class="px-4 py-4">{{ trans('main.actions') }}</th>
        </tr>
        </thead>
        <tbody v-for="(value, key) in tasks">
        <tr class="stone-table-row">
          <td class="px-3 py-4">{{ value.command }}</td>
          <td class="px-3 py-4">{{ value.execute_date }}</td>
          <td class="px-3 py-4">{{ humanRepeatText(value.repeat) }}</td>
          <td class="px-3 py-4">
            <GButton v-on:click="editTask(key)" color="blue" size="small" class="mr-1">
              <i class="fas fa-edit"></i>
              <span class="hidden lg:inline">&nbsp;{{ trans('main.edit') }}</span>
            </GButton>
            <GButton v-on:click="deleteTask(key)" color="red" size="small">
              <i class="fas fa-trash"></i>
              <span class="hidden lg:inline">&nbsp;{{ trans('main.delete') }}</span>
            </GButton>
          </td>
        </tr>
        </tbody>
      </table>

      <n-modal
            v-model:show="modalEnabled"
            class="custom-card"
            preset="card"
            :title="modalTitle"
            :bordered="false"
            style="width: 600px"
            :segmented="segmented"
        >
            <div>
                <form>
                    <div class="mb-3">
                        <label for="task" class="control-label">{{ trans('servers_tasks.task') }}</label>

                        <n-select v-model:value="command" :options="options" v-on:update="formChange" />

                        <span v-if="errors['command']" class="help-block">
                                    <strong class="text-red-600">{{ errors['command'] }}</strong>
                                </span>
                    </div>

                    <div class="mb-3">
                        <n-date-picker
                            v-model:formatted-value="taskDate"
                            value-format="yyyy-MM-dd HH:mm:ss"
                            type="datetime"
                            v-on:update="formChange"
                            clearable
                        /><br>

                        <span v-if="errors['taskDate']" class="help-block">
                                    <strong class="text-red-600">{{ errors['taskDate'] }}</strong>
                                </span>
                    </div>

                    <div class="relative block mb-2">
                          <n-radio
                              :checked="taskRepeatRadio === '1' || taskRepeatRadio === 1"
                              @change="onTaskRepeatRadioChange"
                              name="repeat"
                              :label="trans('servers_tasks.no_repeat')"
                              value="1"
                          />
                    </div>

                    <div class="relative block mb-2">
                        <n-radio
                            :checked="taskRepeatRadio === '0' || taskRepeatRadio === 0"
                            @change="onTaskRepeatRadioChange"
                            name="repeat"
                            :label="trans('servers_tasks.endlessly_repeat')"
                            value="0"
                        />
                    </div>

                    <div class="relative block mb-2">
                        <n-radio
                            :checked="taskRepeatRadio === ''"
                            @change="onTaskRepeatRadioChange"
                            name="repeat"
                            :label="trans('servers_tasks.custom_repeat')"
                            value=""
                        />
                    </div>

                    <div class="mb-3">
                        <label for="repeat" class="control-label">{{ trans('servers_tasks.repeat_num') }}</label>
                        <n-input-number
                            v-model:value="taskRepeatInput"
                            :disabled="taskRepeatRadio !== ''"
                            id="repeat"
                            :min="1"
                            :max="255"
                        />

                        <span v-if="errors['taskRepeatInput']" class="help-block">
                                    <strong class="text-red-600">{{ errors['taskRepeatInput'] }}</strong>
                                </span>
                    </div>

                    <div class="mb-3">
                        <label class="control-label">{{ trans('servers_tasks.repeat_period') }}</label>

                        <div class="flex flex-wrap ">
                            <div class="md:w-1/3 pr-4 pl-4">
                                <n-input-number
                                    v-model:value="taskRepeatPeriod"
                                    :disabled="repeat === 1"
                                    v-on:update="formChange"
                                    id="repeat_period"
                                    name="repeat_period"
                                    type="number"
                                    :min="repeatMin"
                                />
                            </div>

                            <div class="md:w-2/3 pr-4 pl-4">
                                <n-select
                                    v-model:value="taskRepeatUnit"
                                    :disabled="repeat === 1"
                                    v-on:update="formChange"
                                    :options="unitOptions"
                                />
                            </div>
                        </div>

                        <span v-if="errors['taskRepeatPeriod']" class="help-block">
                                    <strong class="text-red-600">{{ errors['taskRepeatPeriod'] }}</strong>
                                </span>

                    </div>

                </form>
            </div>

            <template #footer>
                <GButton color="blue" v-on:click="sendTaskForm" class="mr-1">
                  {{ buttonName }}
                </GButton>
                <GButton color="white" v-on:click="hideModal">
                  {{ trans('main.close') }}
                </GButton>
            </template>
      </n-modal>
    </div>
</template>

<script>
    import { mapState } from 'vuex';
    import { ref } from "vue";
    import _ from 'lodash';
    import { pluralize, trans } from '../../i18n/i18n'
    import GButton from "../../components/GButton.vue";

    const REPEAT_ENDLESSLY          = 0;
    const REPEAT_ONCE               = 1;

    const RADIO_REPEAT_CUSTOM       = '';

    export default {
      components: {GButton},
        props: {
            serverId: Number,
            privileges: {
                start: true,
                stop: true,
                restart: true,
                update: true
            },
        },
        data: function () {
            return {
                command: ref(''),
                taskDate: ref('2007-06-30 12:08:55'),
                taskRepeatInput: ref(1),
                taskRepeatRadio: ref(0),
                taskRepeatPeriod: ref(0),
                taskRepeatUnit: ref('hours'),

                selectedTaskIndex: ref(null),
                errors: {},

                modalEnabled: ref(false),
                modalTitle: ref(''),
                buttonName: this.trans('main.create'),

                segmented: {
                    content: 'soft',
                    footer: 'soft'
                },
            }
        },
        methods: {
          trans,
            createTask() {
                this.command = null;
                this.taskDate = null;
                this.taskRepeatInput = 1;
                this.taskRepeatPeriod = 1;
                this.taskRepeatUnit = 'hours';

                this.selectedTaskIndex = null;

                this.buttonName = this.trans('main.create');
                this.modalTitle = this.trans('servers_tasks.new_task');

                this.showModal();
            },
            editTask(index) {
                this.command = this.tasks[index].command;
                this.taskDate = this.tasks[index].execute_date;
                this.taskRepeatInput = '';
                this.repeat = this.tasks[index].repeat;

                const repeat = this.tasks[index].repeat_period.split(' ');

                this.taskRepeatPeriod = repeat[0];
                this.taskRepeatUnit = this.repeatUnitPlural(repeat[1]);

                this.selectedTaskIndex = index;

                this.buttonName = this.trans('main.save');
                this.modalTitle = this.trans('servers_tasks.edit_task');

                this.showModal();
            },
            sendTaskForm() {
                if (!this.checkForm()) {
                    return;
                }

                const form = {
                    server_id: this.serverId,
                    command: this.command,
                    execute_date: this.taskDate,
                    repeat: this.repeat,
                };

                form.repeat_period = form.repeat !== 1
                    ? this.taskRepeatPeriod + ' ' + this.taskRepeatUnit
                    : '';

                if (this.selectedTaskIndex === null) {
                    this.$store.dispatch('servers/storeTask', form)
                        .then(() => {
                            this.hideModal();
                        }).catch((e) => {
                            this.hideModal();

                            _.has(e, 'response.data.message')
                                ? gameap.alert(e.response.data.message)
                                : gameap.alert(e);
                        });
                } else {
                    this.$store.dispatch('servers/updateTask', {
                            taskIndex: this.selectedTaskIndex,
                            task: form
                        }).then(() => {
                            this.hideModal();
                        }).catch((e) => {
                            this.hideModal();

                            _.has(e, 'response.data.message')
                                ? gameap.alert(e.response.data.message)
                                : gameap.alert(e);
                        });
                }
            },
            checkForm() {
                this.resetErrors();
                let error = false;

                if (!this.command) {
                    error = true;
                    this.errors.task = this.trans('servers_tasks.errors.empty_task_command');
                }

                if (!this.taskDate) {
                    error = true;
                    this.errors.taskDate = this.trans('servers_tasks.errors.empty_task_date');
                }

                if (this.taskRepeatRadio === RADIO_REPEAT_CUSTOM
                    && (isNaN(parseInt(this.taskRepeatInput))
                        || this.taskRepeatInput < 1
                        || this.taskRepeatInput > 255)
                ) {
                    error = true;
                    this.errors.taskRepeatInput = this.trans('servers_tasks.errors.invalid_repeat_value');
                }

                if (this.repeat !== REPEAT_ONCE) {
                    if (!this.taskRepeatUnit) {
                        error = true;
                        this.errors.taskRepeatPeriod = this.trans('servers_tasks.errors.empty_period_unit');
                    } else if (!this.taskRepeatPeriod) {
                        error = true;
                        this.errors.taskRepeatPeriod = this.trans('servers_tasks.errors.empty_period');
                    } else if (this.taskRepeatUnit === 'minutes' && this.taskRepeatPeriod < 10) {
                        error = true;
                        this.errors.taskRepeatPeriod = this.trans('servers_tasks.errors.minimum_period');
                    }
                }

                return !error;
            },
            resetErrors() {
                this.errors = {
                    command: null,
                    taskDate: null,
                    taskRepeatRadio: null,
                    taskRepeatInput: null,
                    taskRepeatPeriod: null,
                };
            },
            onTaskRepeatRadioChange(e) {
                this.taskRepeatRadio = e.target.value;
                this.formChange();
            },
            formChange() {
                this.resetErrors();
            },
            deleteTask(taskIndex) {
                gameap.confirm(this.trans('servers_tasks.confirm_remove'), () => {
                    this.$store.dispatch('servers/destroyTask', taskIndex);
                })
            },
            humanRepeatText(repeatInt) {
                if (repeatInt === REPEAT_ENDLESSLY) {
                    return this.trans('servers_tasks.endlessly');
                }

                if (repeatInt === REPEAT_ONCE) {
                    return this.trans('servers_tasks.once');
                }

                return repeatInt;
            },
            repeatUnitPlural(unit) {
                switch (unit) {
                    case 'm':
                    case 'min':
                    case 'minute':
                        return 'minutes';

                    case 'h':
                    case 'hour':
                        return 'hours';

                    case 'd':
                    case 'day':
                        return 'days';

                    case 'w':
                    case 'week':
                        return 'weeks';

                    case 'month':
                        return 'months';

                    case 'y':
                    case 'year':
                        return 'years';
                }

                return unit;
            },
            showModal() {
                this.modalEnabled = true;
            },
            hideModal() {
                this.modalEnabled = false;
            },
        },
        computed: {
            ...mapState({
                tasks: state => state.servers.tasks,
            }),
            repeat: {
                get() {
                    return parseInt(this.taskRepeatRadio === RADIO_REPEAT_CUSTOM
                        ? this.taskRepeatInput
                        : this.taskRepeatRadio);
                },
                set(repeat) {
                    repeat = parseInt(repeat);
                    if (repeat <= 0) {
                        this.taskRepeatRadio = 0;
                    } else if (repeat === REPEAT_ONCE) {
                        this.taskRepeatRadio = 1;
                    } else {
                        this.taskRepeatRadio = RADIO_REPEAT_CUSTOM;
                        this.taskRepeatInput = repeat;
                    }
                }
            },
            repeatMin: {
                get() {
                    if (this.taskRepeatUnit === 'minutes') {
                        return 10;
                    } else {
                        return 1;
                    }
                }
            },
            unitOptions: {
                get() {
                    return [
                        {
                            label: pluralize('minute', parseInt(this.taskRepeatPeriod)),
                            value: 'minutes',
                        },
                        {
                            label: pluralize('hour', parseInt(this.taskRepeatPeriod)),
                            value: 'hours',
                        },
                        {
                            label: pluralize('day', parseInt(this.taskRepeatPeriod)),
                            value: 'days',
                        },
                        {
                            label: pluralize('week', parseInt(this.taskRepeatPeriod)),
                            value: 'weeks',
                        },
                        {
                            label: pluralize('month', parseInt(this.taskRepeatPeriod)),
                            value: 'months',
                        },
                        {
                            label: pluralize('year', parseInt(this.taskRepeatPeriod)),
                            value: 'years',
                        },
                    ];
                }
            },
            options: {
                get() {
                    let result = [];

                    if (this.privileges.restart) {
                        result.push({
                            label: trans('servers.restart'),
                            value: "restart",
                        })
                    }

                    if (this.privileges.start) {
                        result.push({
                            label: trans('servers.start'),
                            value: "start",
                        })
                    }

                    if (this.privileges.stop) {
                        result.push({
                            label: trans('servers.stop'),
                            value: "stop",
                        })
                    }

                    if (this.privileges.update) {
                        result.push({
                            label: trans('servers.update'),
                            value: "update",
                        })
                    }

                    if (this.privileges.update) {
                        result.push({
                            label: trans('servers.reinstall'),
                            value: "reinstall",
                        })
                    }

                    return result;
                }
            }
        },
        mounted() {
            this.$store.dispatch('servers/setServerId', this.serverId);
            this.$store.dispatch('servers/fetchTasks');
        }
    }
</script>
