<template>
  <GIcon :name="iconName" />
</template>

<script setup>
import { computed } from 'vue'
import GIcon from './GIcon.vue'

const icons = {
  '7d2d': '7',
  '7days2die': '7',
  ark: 'game-ark-survival-evolved',
  arma2: 'game-arma-2',
  arma2oa: 'game-arma-2',
  arma3: 'game-arma-3',
  blackmesasource: 'game-black-mesa',
  bms: 'game-black-mesa',
  counterstrike: 'game-counter-strike-1',
  cs15: 'game-counter-strike-1',
  cs2: 'game-counter-strike',
  csgo: 'game-counter-strike',
  css: 'game-counter-strike-source',
  cssource: 'game-counter-strike-source',
  cssv34: 'game-counter-strike-source',
  cstrike: 'game-counter-strike-1',
  czero: 'game-counter-strike-1',
  dod: 'game-day-of-defeat',
  dods: 'game-day-of-defeat',
  dontstarve: 'game-dont-starve',
  dontstarvetogether: 'game-dont-starve',
  dst: 'game-dont-starve',
  enshrouded: 'game-enshrouded',
  et: 'game-etlegacy',
  etl: 'game-etlegacy',
  etlegacy: 'game-etlegacy',
  factorio: 'game-factorio',
  fivem: 'game-fivem',
  garrysmod: 'game-garrys-mod',
  grandtheftauto: 'game-grand-theft-auto',
  gta: 'game-grand-theft-auto',
  halflife: 'game-half-life',
  hurtworld: 'game-hurtworld',
  l4d2: 'game-left-4-dead',
  l4d: 'game-left-4-dead',
  minecraft: 'game-minecraft',
  mta: 'game-grand-theft-auto',
  op4: 'game-half-life',
  palworld: 'game-palworld',
  pmmp: 'game-minecraft-creeper',
  projectzomboid: 'game-zomboid',
  pz: 'game-zomboid',
  qw: 'game-quake',
  q1: 'game-quake',
  quake: 'game-quake',
  q2: 'game-quake-2',
  quake2: 'game-quake-2',
  q3: 'game-quake-3',
  quake3: 'game-quake-3',
  q4: 'game-quake',
  quake4: 'game-quake',
  rust: 'game-rust',
  samp: 'game-rockstar',
  teamfortress2: 'game-team-fortress-2',
  teamspeak3: 'teamspeak',
  teamspeak: 'teamspeak',
  tf2: 'game-team-fortress-2',
  valve: 'game-half-life',
  wolfet: 'game-etlegacy',
  zomboid: 'game-zomboid',
}

const commonIcons = [
  'gamepad',
  'dice-one',
  'dice-two',
  'dice-three',
  'dice-four',
  'dice-five',
  'dice-six',
  'dice',
  'puzzle-piece',
]

const props = defineProps({
  game: {
    type: String,
    default: 'minecraft',
  },
})

const iconName = computed(() => {
  if (icons[props.game] !== undefined) {
    return icons[props.game]
  }

  return defineCommonIcon(props.game)
})

function defineCommonIcon(game) {
  return commonIcons[checksum(game)]
}

function checksum(data) {
  let c = 0
  for (let i = 0; i < data.length; i++) {
    c += data.charCodeAt(i)
  }

  return c % commonIcons.length
}
</script>
