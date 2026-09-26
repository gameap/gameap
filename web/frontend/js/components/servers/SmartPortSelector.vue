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
import { ref, computed, watch, onMounted, defineModel } from 'vue';
import { storeToRefs } from 'pinia'
import { useNodeStore } from '@/store/node'
import { useGameStore } from '@/store/game'
import { useServerStore } from '@/store/server'
import {
  NFormItem,
  NInputNumber
} from 'naive-ui';
import { trans } from '@/i18n/i18n';
import { PORT_RANGE_KEY, findFreePort, inPortRange, parsePortRange } from '@/parts/portRange';

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
  'rust': [0, 1],
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
const { nodeId: dsId, busyPorts, node } = storeToRefs(nodeStore)
const { gameCode } = storeToRefs(gameStore)
const { formIp: selectedIp } = storeToRefs(serverStore)

const serverPort = defineModel('serverPort')
const queryPort = defineModel('queryPort')
const rconPort = defineModel('rconPort')

const serverPortWarning = ref('')

// The node store is shared with the node pages, so a node another view left
// there must not lend its pool to this form.
const portRanges = computed(() => {
  if (Number(node.value?.id) !== Number(dsId.value)) {
    return []
  }

  return parsePortRange(node.value?.metadata?.[PORT_RANGE_KEY]) ?? []
})

function setPorts() {
  if (props.initialServerIp === selectedIp.value) {
    serverPort.value = parseInt(props.initialServerPort) || 27015;

    const portDiff = getPortDiff();
    queryPort.value = parseInt(props.initialQueryPort) || serverPort.value + portDiff[0];
    rconPort.value = parseInt(props.initialRconPort) || serverPort.value + portDiff[1];

    return
  }

  const start = DEFAULT_PORTS[getExistsPortGameCode()];
  const [queryDiff, rconDiff] = getPortDiff();
  const portsOf = (port) => [port, port + queryDiff, port + rconDiff];
  const isFree = (port) => portsOf(port).every((p) => p <= 65535 && !isBusy(selectedIp.value, p));
  const pool = portRanges.value;

  // A full pool falls back to counting up from the game's default port: the
  // API accepts a port outside the pool and still refuses a taken one.
  const pooled = pool.length > 0
      ? findFreePort(pool, start, (port) => isFree(port) && portsOf(port).every((p) => inPortRange(pool, p)))
      : null;

  serverPort.value = pooled ?? findFreePort([], start, isFree) ?? start;

  // The serverPort watcher skips a port that did not change, yet another game
  // may still need other query and RCON offsets.
  correctPorts();
}

function fetchNodeDetails() {
  nodeStore.fetchBusyPorts(checkPorts)

  // Only the pool is read from the node; without it the form picks as before.
  if (dsId.value > 0) {
    nodeStore.fetchNode().catch(() => {})
  }
}

function correctPorts() {
  const portDiff = getPortDiff();

  queryPort.value = serverPort.value + portDiff[0];
  rconPort.value = serverPort.value + portDiff[1];
}

function getExistsPortGameCode() {
  return DEFAULT_PORTS.hasOwnProperty(gameCode.value) ? gameCode.value : 'default';
}

function getPortDiff() {
  return PORT_DIFF[gameCode.value] || PORT_DIFF['default'];
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
  fetchNodeDetails()
});

watch(dsId, () => {
  fetchNodeDetails()
});

// Picking needs the busy ports and the node's pool, which may arrive after the
// address is chosen; pick again once either lands.
watch([busyPorts, portRanges], () => {
  if (selectedIp.value) {
    setPorts();
  }
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

</script>
