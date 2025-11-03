<template>
    <div>
        <input type="hidden" :name="dsIdFieldName" v-model="nodeIdModel" />
        <input type="hidden" :name="serverIpFieldName" v-model="ipModel" />

        <n-form-item :path="nodeIdPath">
          <n-select
              v-model:value="nodeIdModel"
              :disabled="nodeSelectDisabled"
              :options="nodesOptions"
              :placeholder="trans('labels.ds_id')"
          />
        </n-form-item>

        <n-form-item :path="ipPath" :show-label="false">
          <n-select
              v-model:value="ipModel"
              :disabled="!nodeIdModel"
              :options="ipListOptions"
              :placeholder="trans('labels.ip')"
          />
        </n-form-item>
    </div>
</template>

<script setup>
  import { computed, watch, onUnmounted } from 'vue'
  import { useStore } from 'vuex'
  import {NFormItem} from "naive-ui"

  const props = defineProps({
    dsList: Object,
    serverIpFieldName: {
      type: String,
      default: 'server_ip',
    },
    dsIdFieldName: {
      type: String,
      default: 'ds_id',
    },
    nodeSelectDisabled: {
      type: Boolean,
      default: false,
    },
    nodeIdPath: "nodeId",
    ipPath: "ip",
  });

  const nodeIdModel = defineModel('nodeId')
  const ipModel = defineModel('ip')

  const store = useStore();

  const ipList = computed(() => store.state.dedicatedServers.ipList);

  const nodesOptions = computed(() => {
    return Object.entries(props.dsList).map(([id, name]) => ({ value: Number(id), label: name }));
  });

  const ipListOptions = computed(() => {
    return ipList.value.map((ip) => ({ value: ip, label: ip }));
  });

  watch(ipList, (list) => {
    if (list.length >= 1 && !list.includes(ipModel.value)) {
      ipModel.value = list[0];
    }
  });

  watch(nodeIdModel, (val) => {
    store.dispatch('dedicatedServers/setDsId', nodeIdModel.value);
    store.dispatch('dedicatedServers/fetchIpList');
  });

  watch(ipModel, (val) => {
    store.dispatch('servers/setIp', ipModel.value)
  });

  onUnmounted(() => {
    store.dispatch('dedicatedServers/resetDsId');
  });
</script>
