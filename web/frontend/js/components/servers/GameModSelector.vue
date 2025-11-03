<template>
    <div>
        <div class="mb-3">
            <input type="hidden" name="game_id" v-model="gameModel">

            <n-form-item :label="trans('labels.game_id')" :path="gamePath">
              <n-select
                  filterable
                  :disabled="gameSelectDisabled"
                  v-model:value="gameModel"
                  :options="gamesOptions"
                  :render-label="renderGameLabel"
              />
            </n-form-item>

        </div>

        <div class="mb-3">
            <input type="hidden" name="game_mod_id" v-model="gameModModel">

            <n-form-item :label="trans('labels.game_mod_id')" :path="gameModPath">
              <n-select
                  filterable
                  v-model:value="gameModModel"
                  :disabled="!gameModel"
                  :options="gameModOptions"
              />
            </n-form-item>
        </div>
    </div>
</template>

<script setup>
  import { computed, h, watch, defineModel, onUnmounted } from 'vue';
  import { useStore } from 'vuex';
  import {trans} from "@/i18n/i18n";
  import {NFormItem} from "naive-ui";
  import GameIcon from "../GameIcon.vue";

  const props = defineProps({
    games: Object,
    gamePath: "game",
    gameModPath: "gameMod",
    gameSelectDisabled: false,
  });

  const gameModel = defineModel('game')
  const gameModModel = defineModel('gameMod')

  const store = useStore();

  const gameModsList = computed(() => store.state.gameMods.gameModsList);

  onUnmounted(() => {
    store.dispatch('gameMods/setGameMod', null)
    store.dispatch('games/setGameCode', null)
  });

  const renderGameLabel = (option) => {
    return [
      h(GameIcon, {game: option.value, class: 'mr-2'}),
      option.label,
    ]
  }

  const gamesOptions = computed(() => {
    return Object.entries(props.games).map(([gameCode, gameName]) => ({ value: gameCode, label: gameName }));
  });

  const gameModOptions = computed(() => {
    return gameModsList.value.map((gameMod) => ({ value: Number(gameMod.id), label: gameMod.name }));
  });

  watch(gameModel, () => {
    store.dispatch('games/setGameCode', gameModel.value)
    store.dispatch('gameMods/fetchGameModsList', gameModel.value);
  });

  watch(gameModModel, () => {
    store.dispatch('gameMods/setGameMod', gameModModel.value)
  });

  watch(gameModsList, (val) => {
    let mod = null
    if (val.length > 0) {
      mod = val[0].id
    }
    const found = val.find((element) => element.id === gameModModel.value);
    if (found) {
      mod = gameModModel.value;
    }

    gameModModel.value = mod;
  });
</script>

