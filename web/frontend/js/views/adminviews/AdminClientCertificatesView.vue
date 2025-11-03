<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <GButton color="green" size="middle" class="mb-5 mr-1" v-on:click="onClickCreate()">
    <i class="fa fa-plus-square mr-0.5"></i>
    <span>{{ trans('client_certificates.upload')}}</span>
  </GButton>

  <n-data-table
      ref="tableRef"
      :bordered="false"
      :single-line="true"
      :columns="columns"
      :data="clientCertificatesData"
      :loading="loading"
      :pagination="pagination"
  >
    <template #loading>
      <Loading />
    </template>
  </n-data-table>

  <n-modal
      v-model:show="createCertificateModalEnabled"
      class="custom-card"
      preset="card"
      :title="trans('client_certificates.title_upload')"
      :bordered="false"
      style="width: 600px"
      :segmented="{content: 'soft', footer: 'soft'}"
  >
    <CreateCertificateForm
        v-model="createCertificateModel"
        v-model:certificateFile="createCertificateCertificateFile"
        v-model:privateKeyFile="createCertificatePrivateKeyFile"
        v-on:create="onCreate"
    />
  </n-modal>

  <n-modal
      v-model:show="viewCertificateModalEnabled"
      class="custom-card"
      preset="card"
      :title="trans('client_certificates.title_view')"
      :bordered="false"
      style="width: 800px"
      :segmented="{content: 'soft', footer: 'soft'}"
  >
    <n-table :bordered="false" :single-line="true">
      <tbody>
      <tr>
        <td><strong>{{ trans('client_certificates.fingerprint') }}:</strong></td>
        <td>{{ selectedCertificate.fingerprint }}</td>
      </tr>
      <tr>
        <td><strong>{{ trans('client_certificates.signature_type') }}:</strong></td>
        <td>{{ selectedCertificate.info.signature_type }}</td>
      </tr>
      <tr>
        <td><strong>{{ trans('client_certificates.common_name') }}:</strong></td>
        <td>{{ selectedCertificate.info.common_name }}</td>
      </tr>
      <tr>
        <td><strong>{{ trans('client_certificates.country') }}:</strong></td>
        <td>{{ selectedCertificate.info.country }}</td>
      </tr>
      <tr>
        <td><strong>{{ trans('client_certificates.organization') }}:</strong></td>
        <td>{{ selectedCertificate.info.organization }}</td>
      </tr>
      <tr>
        <td><strong>{{ trans('client_certificates.organizational_unit') }}:</strong></td>
        <td>{{ selectedCertificate.info.organizational_unit }}</td>
      </tr>
      <tr>
        <td><strong>{{ trans('client_certificates.email') }}:</strong></td>
        <td>{{ selectedCertificate.info.email }}</td>
      </tr>
      <tr>
        <td><strong>{{ trans('client_certificates.expires') }}:</strong></td>
        <td>{{ selectedCertificate.info.expires }}</td>
      </tr>
      </tbody>
    </n-table>
  </n-modal>
</template>

<script setup>
import GBreadcrumbs from "@/components/GBreadcrumbs.vue"
import {computed, h, ref, onMounted} from "vue"
import _ from "lodash"
import {trans} from "@/i18n/i18n"
import GButton from "@/components/GButton.vue"
import {errorNotification, notification} from "@/parts/dialogs"
import Loading from "@/components/Loading.vue"
import {storeToRefs} from "pinia"
import {useClientCertificatesStore} from "@/store/clientCertificates"
import {
  NTable,
  NDataTable,
  NModal,
} from "naive-ui"
import CreateCertificateForm from "./forms/CreateCertificateForm.vue";

const clientCertificatesStore = useClientCertificatesStore()

const breadcrumbs = computed(() => {
  return [
    {'route':'/', 'text':'GameAP', 'icon': 'gicon gicon-gameap'},
    {'route':{name: 'admin.nodes.index'}, 'text':trans('sidebar.dedicated_servers')},
    {'route':{name: 'admin.client_certificates.index'}, 'text':trans('client_certificates.client_certificates')},
  ]
})

const createColumns = () => {
  return [
    {
      title: trans('client_certificates.fingerprint'),
      key: "fingerprint",
    },
    {
      title: trans('client_certificates.expires'),
      key: "expires",
    },
    {
      title: trans('main.actions'),
      render(row) {
        return [
          h(GButton, {
            color: 'green',
            size: 'small',
            class: 'mr-0.5',
            onClick: () => {onClickShow(row.id)},
          }, [
            h("i", {class: 'fa-solid fa-eye'}),
            h("span", {class: 'hidden lg:inline'}, trans('main.view')),
          ]),
          h(GButton, {
            color: 'red',
            size: 'small',
            text: trans('main.delete'),
            onClick: () => {onClickDelete(row.id)},
          }, [
            h("i", {class: 'fa-solid fa-trash'}),
            h("span", {class: 'hidden lg:inline'}, trans('main.delete')),
          ]),
        ]
      },
    }
  ]
}

const {loading, certificates} = storeToRefs(clientCertificatesStore)

const columns = ref(createColumns())
const pagination = {
  pageSize: 20,
}

onMounted(() => {
  fetchClientCertificates()
})

const fetchClientCertificates = () => {
  clientCertificatesStore.fetchClientCertificates().catch((error) => {
    errorNotification(error);
  })
}

const clientCertificatesData = computed(() => {
  return certificates.value.map(
    (certificate) => {
      return {
        id: certificate.id,
        fingerprint: certificate.fingerprint,
        expires: certificate.expires,
        info: Object.fromEntries(
            Object.entries(certificate.info).map(([k, v]) => [_.camelCase(k), v])
        ),
      };
    },
  )
})

const createCertificateModalEnabled = ref(false)
const createCertificateModel = ref({})
const createCertificateCertificateFile = ref()
const createCertificatePrivateKeyFile = ref()

const onClickCreate = () => {
  createCertificateModel.value = {}
  createCertificateCertificateFile.value = {}
  createCertificatePrivateKeyFile.value = {}

  createCertificateModalEnabled.value = true
}

const onCreate = () => {
  const fields = {
    certificate: createCertificateCertificateFile.value,
    private_key: createCertificatePrivateKeyFile.value,
    private_key_pass: createCertificateModel.value.privateKeyPass ?? '',
  }
  clientCertificatesStore.createClientCertificate(fields).then(() => {
    notification({
      content: trans('client_certificates.upload_success_msg'),
      type: "success",
    }, () => {
      createCertificateModalEnabled.value = false
      fetchClientCertificates()
    })
  }).catch((error) => {
    errorNotification(error);
  })
}

const viewCertificateModalEnabled = ref(false)
const selectedCertificate = ref({})

const onClickShow = (certificateId) => {
  viewCertificateModalEnabled.value = true
  selectedCertificate.value = certificates.value.find(certificate => certificate.id === certificateId)
}

const onClickDelete = (certificateId) => {
  const deleteFunc = () => {
    clientCertificatesStore.deleteClientCertificate(certificateId).then(() => {
      notification({
        content: trans('client_certificates.delete_success_msg'),
        type: "success",
      }, () => {
        fetchClientCertificates()
      })
    }).catch((error) => {
      errorNotification(error);
    })
  }

  window.$dialog.success({
    title: trans('client_certificates.delete_confirm_msg'),
    positiveText: trans('main.yes'),
    negativeText: trans('main.no' ),
    closable: false,
    onPositiveClick: () => {
      deleteFunc()
    },
    onNegativeClick: () => {}
  })
}

</script>