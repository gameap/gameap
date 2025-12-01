<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <UpdateNodeForm
      :loading="loading"
      v-model="nodeUpdateModel"
      v-on:send="onUpdate"
      :client-certificate-options="certificateOptions"
    />
</template>

<script setup>
import GBreadcrumbs from "@/components/GBreadcrumbs.vue"
import {computed, ref, onMounted} from "vue"
import { camelCase, snakeCase } from "lodash-es"
import {trans} from "@/i18n/i18n"
import {useNodeStore} from "@/store/node"
import {useClientCertificatesStore} from "@/store/clientCertificates";
import {errorNotification, notification} from "@/parts/dialogs"
import {useRoute, useRouter} from "vue-router"
import {storeToRefs} from "pinia"
import UpdateNodeForm from "./forms/UpdateNodeForm.vue";

const route = useRoute()
const router = useRouter()

const nodeStore = useNodeStore()
const clientCertificatesStore = useClientCertificatesStore()

const breadcrumbs = computed(() => {
  return [
    {route:'/', text:'GameAP', icon: 'gicon gicon-gameap'},
    {route:{name: 'admin.nodes.index'}, text:trans('dedicated_servers.dedicated_servers')},
    {text: trans('dedicated_servers.edit')},
  ]
})

onMounted(() => {
  nodeStore.setNodeId(route.params.id)
  nodeStore.fetchNode().then(() => {
    nodeUpdateModel.value = Object.fromEntries(
        Object.entries(node.value).map(([k, v]) => [camelCase(k), v])
    );
  }).catch((error) => {
    errorNotification(error)
  })

  clientCertificatesStore.fetchClientCertificates().catch((error) => {
    errorNotification(error)
  })
})

const { node } = storeToRefs(nodeStore)
const { certificates } = storeToRefs(clientCertificatesStore)

const loading = computed(() => {
  return nodeStore.loading.value || clientCertificatesStore.loading.value;
})

const certificateOptions = computed(() => {
  return certificates.value.map((certificate) => {
    return {
      label: certificate.fingerprint,
      value: certificate.id,
    };
  })
})

const nodeUpdateModel = ref({
  name: '',
  description: '',
  location: '',
})

const onUpdate = async () => {
  if (nodeUpdateModel.value.serverCertificateFile) {
    nodeUpdateModel.value.gdaemonServerCert = await nodeUpdateModel.value.serverCertificateFile.text()
  } else {
    nodeUpdateModel.value.gdaemonServerCert = ''
  }

  const fields = Object.fromEntries(
      Object.entries(nodeUpdateModel.value).map(([k, v]) => [snakeCase(k), v])
  );

  nodeStore.saveNode(fields).then(() => {
    notification({
      content: trans('dedicated_servers.update_success_msg'),
      type: "success",
    }, () => {
      router.push({name: 'admin.nodes.index'})
    })
  }).catch((error) => {
    errorNotification(error)
  })
}

</script>