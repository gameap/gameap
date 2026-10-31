<template>
    <div>
      <n-form-item :label="trans('labels.server_port')" :path="serverPortPath">
        <n-input-number
            name="server_port"
            id="server_port"
            :min="1024"
            :max="65535"
            v-model:value="serverPort"
        />
        <template #feedback>
          <span v-if="serverPortWarning" class="help-block"><strong>{{ serverPortWarning }}</strong></span>
        </template>
      </n-form-item>

      <n-form-item :label="trans('labels.query_port')" :path="queryPortPath">
        <n-input-number
            name="query_port"
            type="number"
            id="server_port"
            :min="1024"
            :max="65535"
            v-model:value="queryPort"
        />
      </n-form-item>

      <n-form-item :label="trans('labels.rcon_port')" :path="rconPortPath">
        <n-input-number
            name="rcon_port"
            type="number"
            id="server_port"
            :min="1024"
            :max="65535"
            v-model:value="rconPort"
        />
      </n-form-item>
    </div>
</template>

<script setup>
import { ref, watch, onMounted, defineModel } from 'vue';
import { storeToRefs } from 'pinia'
import { useNodeStore } from '@/store/node'
import { useGameStore } from '@/store/game'
import { useGameListStore } from '@/store/gameList'
import { useGameModStore } from '@/store/gameMod'
import { useServerStore } from '@/store/server'
import {
  NFormItem,
  NInputNumber
} from 'naive-ui';
import { trans } from '@/i18n/i18n';

// Fallbacks for games whose metadata does not set default_port / query_port_offset /
// rcon_port_offset. The game mod metadata overrides the game metadata.
const DEFAULT_PORTS = {
  'ark': 7777,
  'arma2': 2302,
  'arma2ao': 2302,
  'arma3': 2302,
  'cod4': 28960,
  'fivem': 30120,
  'hurtworld': 12871,
  'hytale': 5520,
  'justcause': 7777,
  'minecraft': 25565,
  'mta': 22003,
  'rok': 7350,
  'rust': 28015,
  'samp': 7777,
  'teamspeak3': 9987,
  'quake': 26000,
  'q1': 26000,
  'quake2': 27910,
  'q2': 27910,
  'quake3': 27960,
  'q3': 27960,
  'quake4': 28004,
  'q4': 28004,
  'default': 27015,
};

const PORT_DIFF = {
  'arma2': [0, 0],
  'arma2ao': [0, 0],
  'arma3': [0, 0],
  'cod4': [0, 0],
  'mta': [0, 2],
  'samp': [0, 0],
  'hurtworld': [10, 0],
  'justcause': [0, 0],
  'fivem': [0, 0],
  'ark': [0, 0],
  'rust': [2, 1],
  'minecraft': [0, 1],
  'rok': [0, 0],
  'teamspeak3': [24, 35],
  'default': [0, 0],
};

const props = defineProps({
  initialServerIp: String,
  initialServerPort: Number,
  initialQueryPort: Number,
  initialRconPort: Number,
  game: String,
  serverPortPath: { type: String, default: 'serverPort' },
  rconPortPath: { type: String, default: 'rconPort' },
  queryPortPath: { type: String, default: 'queryPort' },
});

const emit = defineEmits(['update:serverPort', 'update:rconPort', 'update:queryPort']);

const nodeStore = useNodeStore()
const gameStore = useGameStore()
const serverStore = useServerStore()
const gameListStore = useGameListStore()
const gameModStore = useGameModStore()
const { nodeId: dsId, busyPorts } = storeToRefs(nodeStore)
const { gameCode } = storeToRefs(gameStore)
const { games } = storeToRefs(gameListStore)
const { mod: selectedMod } = storeToRefs(gameModStore)
const { formIp: selectedIp } = storeToRefs(serverStore)

const serverPort = defineModel('serverPort')
const queryPort = defineModel('queryPort')
const rconPort = defineModel('rconPort')

const serverPortWarning = ref('')

// The last port picked automatically. A port the user typed is not replaced when the
// game mod (and with it the default port) changes.
const pickedPort = ref(null)

function setPorts() {
  if (props.initialServerIp === selectedIp.value) {
    serverPort.value = parseInt(props.initialServerPort) || 27015;

    const portDiff = getPortDiff();
    queryPort.value = parseInt(props.initialQueryPort) || serverPort.value + portDiff[0];
    rconPort.value = parseInt(props.initialRconPort) || serverPort.value + portDiff[1];

    return
  }

  const defaultPort = getDefaultPort();
  let portCorrect = -1;

  do {
    portCorrect++;
    serverPort.value = defaultPort + portCorrect;
  } while (isBusy(selectedIp.value, defaultPort + portCorrect));

  pickedPort.value = serverPort.value;

  // watch(serverPort) skips an unchanged value, so the offsets are applied here as well.
  correctPorts();
}

function correctPorts() {
  const portDiff = getPortDiff();

  queryPort.value = serverPort.value + portDiff[0];
  rconPort.value = serverPort.value + portDiff[1];
}

function getExistsPortGameCode() {
  return DEFAULT_PORTS.hasOwnProperty(gameCode.value) ? gameCode.value : 'default';
}

function metadataInt(metadata, key) {
  const value = metadata?.[key];
  if (value === undefined || value === null || value === '') {
    return undefined;
  }

  const number = Number(value);

  return Number.isInteger(number) ? number : undefined;
}

function portMetadata(key) {
  const mod = selectedMod.value;
  if (mod && mod.game_code === gameCode.value) {
    const value = metadataInt(mod.metadata, key);
    if (value !== undefined) {
      return value;
    }
  }

  const game = games.value.find((g) => g.code === gameCode.value);

  return metadataInt(game?.metadata, key);
}

function getDefaultPort() {
  return portMetadata('default_port') ?? DEFAULT_PORTS[getExistsPortGameCode()];
}

function getPortDiff() {
  const fallback = PORT_DIFF[gameCode.value] || PORT_DIFF['default'];

  return [
    portMetadata('query_port_offset') ?? fallback[0],
    portMetadata('rcon_port_offset') ?? fallback[1],
  ];
}

function isBusy(serverIp, serverPort) {
  if (typeof props.initialServerIp !== 'undefined' && typeof props.initialServerPort !== 'undefined' && serverIp === props.initialServerIp && serverPort === props.initialServerPort) {
    return false;
  }

  return busyPorts.value.hasOwnProperty(serverIp) && busyPorts.value[serverIp].indexOf(serverPort) !== -1;
}

function checkPorts() {
  if (selectedIp.value === props.initialServerIp && serverPort.value === props.initialServerPort) {
    serverPortWarning.value = '';
  }

  if (isBusy(selectedIp.value, serverPort.value)) {
    serverPortWarning.value = trans('validation.unique', { attribute: trans('labels.server_port') });
  } else {
    serverPortWarning.value = '';
  }
}

onMounted(() => {
  nodeStore.fetchBusyPorts(checkPorts)
});

watch(dsId, () => {
  nodeStore.fetchBusyPorts(checkPorts)
});

watch(serverPort, (newVal, oldVal) => {
  serverPort.value = Number(serverPort.value);
  correctPorts();
  checkPorts();
  emit('update:serverPort', serverPort.value);
});

watch(rconPort, (newVal) => {
  emit('update:rconPort', rconPort.value);
});

watch(queryPort, (newVal) => {
  emit('update:queryPort', queryPort.value);
});

watch(selectedIp, (newIp, oldIp) => {
  if (newIp) {
    setPorts();
  }

  checkPorts();
});

watch(gameCode, () => {
  setPorts();
});

// Metadata arrives with the games list and the selected game mod, possibly after the game
// was chosen.
watch(() => [selectedMod.value?.id, games.value], () => {
  if (!gameCode.value || props.initialServerIp === selectedIp.value) {
    return;
  }

  if (serverPort.value === pickedPort.value) {
    setPorts();

    return;
  }

  correctPorts();
});

</script>
