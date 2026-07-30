<template>
    <div id="rcon-players-component">
        <GButton color="green" size="small" class="mb-2" v-on:click="updatePlayers">
          <GIcon name="sync" />
        </GButton>

        <div class="overflow-x-auto">
        <GTable size="small">
            <thead>
            <tr>
                <th>{{ trans('rcon.player_name') }}</th>
                <th v-if="scoreRow">{{ trans('rcon.player_score') }}</th>
                <th v-if="pingRow">{{ trans('rcon.player_ping') }}</th>
                <th v-if="ipRow">{{ trans('rcon.player_ip') }}</th>
                <th v-if="showActions">{{ trans('main.actions') }}</th>
            </tr>
            </thead>
            <tbody v-for="(value, key) in players">
            <tr>
                <td>{{ value.name }}</td>
                <td v-if="scoreRow">{{ value.score }}</td>
                <td v-if="pingRow">{{ value.ping }}</td>
                <td v-if="ipRow">{{ value.ip }}</td>
                <td v-if="showActions" class="grid grid-cols-2 gap-x-4">
                  <GButton v-if="canKick" color="black" size="small" class="mb-2" v-on:click="openDialog('kick', key)">
                    <i class="gicon gicon-kick mr-1"></i>
                    <span class="hidden lg:inline">{{ trans('rcon.kick') }}</span>
                  </GButton>

                  <GButton v-if="canBan" color="black" size="small" class="mb-2" v-on:click="openDialog('ban', key)">
                    <GIcon name="ban" class="mr-1" />
                    <span class="hidden lg:inline">{{ trans('rcon.ban') }}</span>
                  </GButton>
                </td>
            </tr>
            </tbody>
        </GTable>
        </div>

        <GModal
            v-model:show="modalEnabled"
            :title="dialogTitle"
            style="width: 600px"
        >
            <div>
                <form>
                    <div class="mb-3" v-if="dialogAction === 'ban' || dialogAction === 'kick'">
                        <label for="input-reason" class="control-label">{{ trans('rcon.reason') }}</label>
                        <input v-model.number="form.reason" id="input-reason" type="text" class="form-control">

                        <span v-if="errors['reason']" class="help-block">
                                    <strong class="text-red-600">{{ errors['reason'] }}</strong>
                                </span>
                    </div>

                    <div class="mb-3" v-if="dialogAction === 'ban'">
                        <label for="input-time" class="control-label">{{ trans('rcon.time') }}</label>
                        <input v-model.number="form.time" id="input-time" type="number" class="form-control">

                        <span v-if="errors['time']" class="help-block">
                                    <strong class="text-red-600">{{ errors['time'] }}</strong>
                                </span>
                    </div>

                    <div class="mb-3" v-if="dialogAction === 'message'">
                        <label for="input-mesage" class="control-label">{{ trans('rcon.message') }}</label>
                        <input v-model.number="form.message" id="input-mesage" type="text" class="form-control">

                        <span v-if="errors['message']" class="help-block">
                                    <strong class="text-red-600">{{ errors['message'] }}</strong>
                                </span>
                    </div>
                </form>
            </div>

            <template #footer>
                <button type="button" class="inline-block align-middle text-center select-none border font-normal whitespace-nowrap rounded py-2 px-3 leading-normal no-underline bg-blue-600 text-white hover:bg-blue-600 me-1" v-on:click="send">{{ trans('main.send') }}</button>
                <button type="button" class="inline-block align-middle text-center select-none border font-normal whitespace-nowrap rounded py-2 px-3 leading-normal no-underline bg-stone-600 text-white hover:bg-stone-700" v-on:click="hideModal">{{ trans('main.close') }}</button>

            </template>
        </GModal>
    </div>
</template>

<script>
    import { ref } from "vue"
    import { storeToRefs } from 'pinia'
    import { useServerStore } from '@/store/server'
    import { useServerRconStore } from '@/store/serverRcon'
    import { some, isEmpty } from 'lodash-es'
    import { pluralize, trans } from '@/i18n/i18n'
    import GButton from "../GButton.vue"
    import { GIcon, GModal, GTable } from '@gameap/ui'
    import {errorNotification, notification} from "@/parts/dialogs";

    export default {
        name: "RconPlayers",
      components: {GButton, GIcon},
        props: {
            serverId: Number,
        },
        setup() {
            const serverStore = useServerStore()
            const rconStore = useServerRconStore()
            const { players, rconSupportedFeatures } = storeToRefs(rconStore)

            return {
                serverStore,
                rconStore,
                players,
                rconSupportedFeatures,
            }
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
                this.rconStore.fetchPlayers();
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
                this.rconStore.banPlayer(this.form.player, this.form.reason, this.form.time)
                    .then(() => {
                        this.hideModal();

                        notification(this.trans('rcon.ban_msg_success'))
                    }).catch((e) => {
                        this.hideModal();

                        errorNotification(e);
                    });
            },
            kick() {
                this.rconStore.kickPlayer(this.form.player, this.form.reason)
                    .then(() => {
                        this.hideModal();

                        notification(this.trans('rcon.kick_msg_success'))
                    }).catch((e) => {
                        this.hideModal();

                        errorNotification(e);
                    });
            },
            sendMessage() {
                this.rconStore.sendPlayerMessage(this.form.playerId, this.form.message)
                    .then(() => {
                        this.hideModal();

                        notification(this.trans('rcon.message_msg_success'))
                    }).catch((e) => {
                        this.hideModal();

                        errorNotification(e);
                    });
            }
        },
        computed: {
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
                return some(this.players, player => !isEmpty(player.ip));
            },
            pingRow() {
                return some(this.players, player => !isEmpty(player.ping));
            },
            scoreRow() {
                return some(this.players, player => !isEmpty(player.score));
            },
            canKick() {
                return this.rconSupportedFeatures.playersKick === true;
            },
            canBan() {
                return this.rconSupportedFeatures.playersBan === true;
            },
            showActions() {
                return this.canKick || this.canBan;
            }
        },
        mounted() {
            this.serverStore.setServerId(this.serverId);
            this.rconStore.fetchPlayers();
        }
    }
</script>

<style scoped>

</style>
