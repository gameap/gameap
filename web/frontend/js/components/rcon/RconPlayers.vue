<template>
    <div id="rcon-players-component">
        <GButton color="green" size="small" class="mb-2" v-on:click="updatePlayers">
          <i class="fas fa-sync"></i>
        </GButton>

        <n-table size="small" :bordered="false">
            <thead>
            <tr>
                <th>{{ trans('rcon.player_name') }}</th>
                <th v-if="scoreRow">{{ trans('rcon.player_score') }}</th>
                <th v-if="pingRow">{{ trans('rcon.player_ping') }}</th>
                <th v-if="ipRow">{{ trans('rcon.player_ip') }}</th>
                <th>{{ trans('main.actions') }}</th>
            </tr>
            </thead>
            <tbody v-for="(value, key) in players">
            <tr>
                <td>{{ value.name }}</td>
                <td v-if="scoreRow">{{ value.score }}</td>
                <td v-if="pingRow">{{ value.ping }}</td>
                <td v-if="ipRow">{{ value.ip }}</td>
                <td class="grid grid-cols-2 gap-x-4">
                  <GButton color="black" size="small" class="mb-2" v-on:click="openDialog('kick', key)">
                    <i class="gicon gicon-kick mr-1"></i>
                    <span class="hidden lg:inline">{{ trans('rcon.kick') }}</span>
                  </GButton>

                  <GButton color="black" size="small" class="mb-2" v-on:click="openDialog('ban', key)">
                    <i class="fas fa-ban mr-1"></i>
                    <span class="hidden lg:inline">{{ trans('rcon.ban') }}</span>
                  </GButton>
                </td>
            </tr>
            </tbody>
        </n-table>

        <n-modal
            v-model:show="modalEnabled"
            class="custom-card"
            preset="card"
            :title="dialogTitle"
            :bordered="false"
            style="width: 600px"
            :segmented="segmented"
        >
            <div>
                <form>
                    <div class="mb-3" v-if="dialogAction === 'ban' || dialogAction === 'kick'">
                        <label for="input-reason" class="control-label">{{ trans('rcon.reason') }}</label>
                        <input v-model.number="form.reason" id="input-reason" type="text" class="block appearance-none w-full py-1 px-2 mb-1 leading-normal bg-white text-stone-800 border border-stone-200 rounded">

                        <span v-if="errors['reason']" class="help-block">
                                    <strong class="text-red-600">{{ errors['reason'] }}</strong>
                                </span>
                    </div>

                    <div class="mb-3" v-if="dialogAction === 'ban'">
                        <label for="input-time" class="control-label">{{ trans('rcon.time') }}</label>
                        <input v-model.number="form.time" id="input-time" type="number" class="block appearance-none w-full py-1 px-2 mb-1 leading-normal bg-white text-stone-800 border border-stone-200 rounded">

                        <span v-if="errors['time']" class="help-block">
                                    <strong class="text-red-600">{{ errors['time'] }}</strong>
                                </span>
                    </div>

                    <div class="mb-3" v-if="dialogAction === 'message'">
                        <label for="input-mesage" class="control-label">{{ trans('rcon.message') }}</label>
                        <input v-model.number="form.message" id="input-mesage" type="text" class="block appearance-none w-full py-1 px-2 mb-1 leading-normal bg-white text-stone-800 border border-stone-200 rounded">

                        <span v-if="errors['message']" class="help-block">
                                    <strong class="text-red-600">{{ errors['message'] }}</strong>
                                </span>
                    </div>
                </form>
            </div>

            <template #footer>
                <button type="button" class="inline-block align-middle text-center select-none border font-normal whitespace-no-wrap rounded py-2 px-3 leading-normal no-underline bg-blue-600 text-white hover:bg-blue-600 me-1" v-on:click="send">{{ trans('main.send') }}</button>
                <button type="button" class="inline-block align-middle text-center select-none border font-normal whitespace-no-wrap rounded py-2 px-3 leading-normal no-underline bg-stone-600 text-white hover:bg-stone-700" v-on:click="hideModal">{{ trans('main.close') }}</button>

            </template>
        </n-modal>
    </div>
</template>

<script>
    import { mapState } from 'vuex'
    import { ref } from "vue"
    import _ from 'lodash'
    import { pluralize, trans } from '@/i18n/i18n'
    import GButton from "../GButton.vue"
    import {
      NTable,
      NModal,
    } from "naive-ui"

    export default {
        name: "RconPlayers",
      components: {GButton},
        props: {
            serverId: Number,
        },
        data: function () {
            return {
                dialogAction: null,
                dialogPlayerIndex: null,
                dialogPlayerName: null,
                form: {
                    playerId: null,
                    player: {},
                    reason: null,
                    time: null,
                    message: null,
                },
                errors: {
                    reason: null,
                    time: null,
                    message: null,
                },
                modalEnabled: ref(false),
                segmented: {
                    content: 'soft',
                    footer: 'soft'
                },
            };
        },
        methods: {
            updatePlayers() {
                this.$store.dispatch('rconPlayers/fetchPlayers');
            },
            openDialog(action, playerIndex) {
                this.resetErrors();
                this.resetForm();

                this.dialogAction = action;
                this.dialogPlayerName = this.players[playerIndex].name;

                this.form.playerId = this.players[playerIndex].id;
                this.form.player = this.players[playerIndex];

                this.showModal();
            },
            showModal() {
                this.modalEnabled = true;
            },
            hideModal() {
                this.modalEnabled = false;
            },
            send() {
                if (!this.checkForm()) {
                    return;
                }

                if (this.dialogAction === 'ban') {
                    this.ban();
                }

                if (this.dialogAction === 'kick') {
                    this.kick();
                }

                if (this.dialogAction === 'message') {
                    this.sendMessage();
                }
            },
            checkForm() {
                this.resetErrors();
                let error = false;

                // noinspection FallThroughInSwitchStatementJS
                switch (this.dialogAction) {
                    case 'ban':
                        if (!this.form.time) {
                            error = true;
                            this.errors.time = 'Empty time';
                        }
                    case 'kick':
                        if (!this.form.reason) {
                            error = true;
                            this.errors.reason = 'Empty reason';
                        }
                        break;

                    case 'message':
                        if (!this.form.message) {
                            error = true;
                            this.errors.reason = 'Empty message';
                        }
                        break;
                }

                return !error;
            },
            resetErrors() {
                this.errors = {
                    reason: null,
                    time: null,
                    message: null,
                };
            },
            resetForm() {
                this.form = {
                    playerId: null,
                    reason: null,
                    time: null,
                    message: null,
                };
            },
            ban() {
                this.$store.dispatch('rconPlayers/banPlayer', {
                    player: this.form.player,
                    reason: this.form.reason,
                    time: this.form.time,
                }).then(() => {
                    this.hideModal();

                    gameap.alert(this.trans('rcon.ban_msg_success'))
                }).catch((e) => {
                    this.hideModal();

                    _.has(e, 'response.data.message')
                        ? gameap.alert(e.response.data.message)
                        : gameap.alert(e);
                });
            },
            kick() {
                this.$store.dispatch('rconPlayers/kickPlayer', {
                    player: this.form.player,
                    reason: this.form.reason
                }).then(() => {
                    this.hideModal();

                    gameap.alert(this.trans('rcon.kick_msg_success'))
                }).catch((e) => {
                    this.hideModal();

                    _.has(e, 'response.data.message')
                        ? gameap.alert(e.response.data.message)
                        : gameap.alert(e);
                });
            },
            sendMessage() {
                this.$store.dispatch('rconPlayers/sendMessage', {
                    playerId: this.form.playerId,
                    message: this.form.message
                }).then(() => {
                    this.hideModal();

                    gameap.alert(this.trans('rcon.message_msg_success'))
                }).catch((e) => {
                    this.hideModal();

                    _.has(e, 'response.data.message')
                        ? gameap.alert(e.response.data.message)
                        : gameap.alert(e);
                });
            }
        },
        computed: {
            ...mapState({
                players: state => state.rconPlayers.players,
            }),
            dialogTitle() {
                switch (this.dialogAction) {
                    case 'ban':
                        return this.trans('rcon.modal_title_ban', {player: this.dialogPlayerName});
                    case 'kick':
                        return this.trans('rcon.modal_title_kick', {player: this.dialogPlayerName});
                    case 'message':
                        return this.trans('rcon.modal_title_msg', {player: this.dialogPlayerName});
                }
            },
            ipRow() {
                return _.has(_.head(this.players), 'ip');
            },
            pingRow() {
                return _.has(_.head(this.players), 'ping');
            },
            scoreRow() {
                return _.has(_.head(this.players), 'score');
            }
        },
        mounted() {
            this.$store.dispatch('servers/setServerId', this.serverId);
            this.$store.dispatch('rconPlayers/fetchPlayers');
        }
    }
</script>

<style scoped>

</style>
