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
import { useStore } from 'vuex';
import {
  NFormItem,
  NInputNumber
} from 'naive-ui';
import { trans } from '../../i18n/i18n';

const DEFAULT_PORTS = {
  'arma2': 2302,
  'arma2ao': 2302,
  'arma3': 2302,
  'cod4': 28960,
  'mta': 22003,
  'samp': 7777,
  'hurtworld': 12871,
  'justcause': 7777,
  'fivem': 30120,
  'ark': 7777,
  'rust': 28015,
  'minecraft': 25565,
  'rok': 7350,
  'teamspeak3': 9987,
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
  initialServerPort: String,
  initialQueryPort: String,
  initialRconPort: String,
  game: String,
  serverPortPath: { type: String, default: 'serverPort' },
  rconPortPath: { type: String, default: 'rconPort' },
  queryPortPath: { type: String, default: 'queryPort' },
});

const emit = defineEmits(['update:serverPort', 'update:rconPort', 'update:queryPort']);

const store = useStore();

const serverPort = defineModel('serverPort')
const queryPort = defineModel('queryPort')
const rconPort = defineModel('rconPort')

const serverPortWarning = ref('');

const selectedIp = computed(() => store.state.servers.ip);

const dsId = computed(() => store.state.dedicatedServers.dsId);
const busyPorts = computed(() => store.state.dedicatedServers.busyPorts);
const gameCode = computed(() => store.state.games.gameCode);

function setPorts() {
  if (props.initialServerIp === selectedIp.value) {
    serverPort.value = parseInt(props.initialServerPort) || 27015;
    queryPort.value = parseInt(props.initialQueryPort) || 27015;
    rconPort.value = parseInt(props.initialRconPort) || 27015;
    return
  }

  const gameCode = getExistsPortGameCode();

  let portCorrect = -1;

  do {
    portCorrect++;
    serverPort.value = DEFAULT_PORTS[gameCode] + portCorrect;
  } while (isBusy(selectedIp.value, DEFAULT_PORTS[gameCode] + portCorrect));
}

function correctPorts() {
  const gameCode = getExistsPortGameCode();

  queryPort.value = serverPort.value + PORT_DIFF[gameCode][0];
  rconPort.value = serverPort.value + PORT_DIFF[gameCode][1];
}

function getExistsPortGameCode() {
  return DEFAULT_PORTS.hasOwnProperty(gameCode.value) ? gameCode.value : 'default';
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
  store.dispatch('dedicatedServers/fetchBusyPorts', checkPorts);
});

watch(dsId, () => {
  store.dispatch('dedicatedServers/fetchBusyPorts', checkPorts);
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
  if (oldIp) {
    setPorts();
  }

  checkPorts();
});

watch(gameCode, () => {
  setPorts();
});

</script>
