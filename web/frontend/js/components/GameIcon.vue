<template>
  <i :class="[gameIconClass, $attrs.class]"></i>
</template>

<script setup>
import {computed, defineProps} from 'vue'

const icons = {
  '7d2d': 'fa-solid fa-7',
  '7days2die': 'fa-solid fa-7',
  ark: 'gicon gicon-ark-survival-evolved',
  arma2: 'gicon gicon-arma-2',
  arma2oa: 'gicon gicon-arma-2',
  arma3: 'gicon gicon-arma-3',
  blackmesasource: 'gicon gicon-black-mesa',
  bms: 'gicon gicon-black-mesa',
  counterstrike: 'gicon gicon-counter-strike1',
  cs15: 'gicon gicon-counter-strike1',
  cs2: 'gicon gicon-counter-strike',
  csgo: 'gicon gicon-counter-strike',
  css: 'gicon gicon-counter-strike-source',
  cssource: 'gicon gicon-counter-strike-source',
  cssv34: 'gicon gicon-counter-strike-source',
  cstrike: 'gicon gicon-counter-strike1',
  czero: 'gicon gicon-counter-strike1',
  dontstarve: 'gicon gicon-dont-starve',
  dontstarvetogether: 'gicon gicon-dont-starve',
  dst: 'gicon gicon-dont-starve',
  fivem: 'gicon gicon-fivem',
  garrysmod: 'gicon gicon-garrys-mod',
  gta: 'gicon gicon-grand-theft-auto',
  grandtheftauto: 'gicon gicon-grand-theft-auto',
  hurtworld: 'gicon gicon-hurtworld',
  l4d: 'gicon gicon-left-4-dead',
  l4d2: 'gicon gicon-left-4-dead',
  halflife: 'gicon gicon-half-life',
  minecraft: 'gicon gicon-minecraft',
  mta: 'gicon gicon-grand-theft-auto',
  op4: 'gicon gicon-half-life',
  pmmp: 'gicon gicon-minecraft-creeper',
  rust: 'gicon gicon-rust',
  samp: 'gicon gicon-rockstar',
  teamfortress2: 'gicon gicon-team-fortress-2',
  teamspeak3: 'fa-brands fa-teamspeak',
  teamspeak: 'fa-brands fa-teamspeak',
  tf2: 'gicon gicon-team-fortress-2',
  valve: 'gicon gicon-half-life',
}

const commonIcons = [
  "fa-solid fa-gamepad",
  "fa-solid fa-dice-one",
  "fa-solid fa-dice-two",
  "fa-solid fa-dice-three",
  "fa-solid fa-dice-four",
  "fa-solid fa-dice-five",
  "fa-solid fa-dice-six",
  "fa-solid fa-dice",
  "fa-solid fa-puzzle-piece",
]

const props = defineProps({
  game: 'minecraft',
  search: '',
})

const gameIconClass = computed(() => {
  if (icons[props.game] === undefined) {
    return defineCommonIcon(props.game)
  }

  return icons[props.game]
})

const defineCommonIcon = (game) => {
  if (icons[game] === undefined) {
    return commonIcons[checksum(game)]
  }
}

function checksum(data) {
  let c = 0;

  for (let i = 0; i < data.length; i++) {
    c += data.charCodeAt(i);
  }
  return c % commonIcons.length;
}

</script>