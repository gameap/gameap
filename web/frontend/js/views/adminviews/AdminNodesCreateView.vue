<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <UpdateNodeForm
      :loading="loading"
      v-model="nodeCreateModel"
      v-on:send="onCreate"
      :client-certificate-options="certificateOptions"
  >
    <template #button>
      <GButton class="mt-2" color="green" v-on:click="onCreate">
        <i class="fa-regular fa-plus-square mr-0.5"></i>
        <span class="hidden lg:inline">&nbsp;{{ trans('main.create') }}</span>
      </GButton>
    </template>
  </UpdateNodeForm>

  <CreateNodeModal
      :link="autoSetupData.link"
      :token="autoSetupData.token"
      :host="autoSetupData.host"
  />
</template>

<script setup>
import GBreadcrumbs from "../../components/GBreadcrumbs.vue"
import {computed, ref, onMounted} from "vue"
import {trans} from "@/i18n/i18n"
import {useClientCertificatesStore} from "@/store/clientCertificates";
import {errorNotification, notification} from "@/parts/dialogs"
import {useRoute, useRouter} from "vue-router"
import {storeToRefs} from "pinia"
import _ from "lodash"
import UpdateNodeForm from "./forms/UpdateNodeForm.vue";
import GButton from "../../components/GButton.vue";
import {useNodeListStore} from "@/store/nodeList";

const route = useRoute()
const router = useRouter()

const nodeListStore = useNodeListStore()
const clientCertificatesStore = useClientCertificatesStore()

const breadcrumbs = computed(() => {
  return [
    {route:'/', text:'GameAP', icon: 'gicon gicon-gameap'},
    {route:{name: 'admin.nodes.index'}, text:trans('dedicated_servers.dedicated_servers')},
    {text: trans('dedicated_servers.create')},
  ]
})

onMounted(() => {
  clientCertificatesStore.fetchClientCertificates().catch((error) => {
    errorNotification(error)
  })

  nodeListStore.fetchAutoSetupData().catch((error) => {
    errorNotification(error)
  })
})

const { certificates } = storeToRefs(clientCertificatesStore)
const { autoSetupData } = storeToRefs(nodeListStore)

const loading = computed(() => {
  return clientCertificatesStore.loading.value;
})

const certificateOptions = computed(() => {
  return certificates.value.map((certificate) => {
    return {
      label: certificate.fingerprint,
      value: certificate.id,
    };
  })
})

const nodeCreateModel = ref({
  name: '',
  description: '',
  location: '',
})

const onCreate = async () => {
  if (nodeCreateModel.value.serverCertificateFile) {
    nodeCreateModel.value.gdaemonServerCert = await nodeCreateModel.value.serverCertificateFile.text()
  } else {
    nodeCreateModel.value.gdaemonServerCert = ''
  }

  const fields = Object.fromEntries(
      Object.entries(nodeCreateModel.value).map(([k, v]) => [_.snakeCase(k), v])
  );

  nodeListStore.createNode(fields).then(() => {
    notification({
      content: trans('dedicated_servers.create_success_msg'),
      type: "success",
    }, () => {
      router.push({name: 'admin.nodes.index'})
    })
  }).catch((error) => {
    errorNotification(error)
  })
}
</script>