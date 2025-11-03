<template>
  <div :class="$attrs.class">
    <n-form
        label-placement="top"
        label-width="auto"
        ref="formRef"
        :model="form"
        :rules="rules"
    >
      <Loading v-if="loading"></Loading>
      <div class="mb-4" :class="loading ? 'hidden' : ''">
        <n-form-item :label="trans('labels.name')" path="name">
          <n-input
              v-model:value="form.name"
              type="text"
          />
        </n-form-item>


        <n-form-item :label="trans('tokens.abilities')" path="abilities">
          <div class="w-full mb-2">
            <div class="m-4" v-for="(abilitiesList, abilityGroupName) in abilities">
              <n-checkbox v-model:checked="groupsChecked[abilityGroupName]" @update:checked="onCheckGroup(abilityGroupName)">
                {{ abilityGroupName }}
              </n-checkbox>

              <n-checkbox-group v-model:value="form.abilities" @update:value="onCheckItem(abilityGroupName)">
                <ul>
                  <li class="ml-4 m-2" v-for="(abilityName, ability) in abilitiesList">
                    <div class="flex flex-wrap">
                      <n-checkbox class="md:w-1/3" :value="ability" :label="ability" />
                      <div class="md:w-2/3">
                        {{ abilityName }}
                      </div>
                    </div>
                  </li>
                </ul>
              </n-checkbox-group>
            </div>
          </div>
        </n-form-item>
      </div>
    </n-form>

    <GButton color="green" v-on:click="onClickGenerate">
      <i class="fa-regular fa-square-plus"></i>
      <span class="hidden lg:inline">&nbsp;{{ trans('main.generate') }}</span>
    </GButton>
  </div>
</template>

<script setup>
import {ref, defineModel, defineProps} from "vue"
import {trans} from "../../i18n/i18n";
import GButton from "../../components/GButton.vue";
import {
  NCheckbox,
  NCheckboxGroup,
  NForm,
  NFormItem,
  NInput,
} from "naive-ui"
import Loading from "@/components/Loading.vue";
import {isArrayNotEmptyValidator, requiredValidator} from "@/parts/validators";
import _ from "lodash";

const props = defineProps({
  loading: {
    type: Boolean,
    required: false,
  },
  abilities: {
    type: Object,
    required: true,
  },
})

const formRef = ref({})
const form = defineModel({
  name: '',
  abilities: [],
})

const groupsChecked = ref({})

const rules = {
  name: {
    required: true,
    validator: requiredValidator(trans('labels.name')),
  },
  abilities: {
    required: true,
    validator: (rule, value) => {
      if (_.isEmpty(value)) {
        return new Error(
            trans('tokens.validation.abilities_required')
        )
      }
    }
  },
}

const onCheckGroup = (group) => {
  for (const [ability] of Object.entries(props.abilities[group])) {
    if (groupsChecked.value[group]) {
      if (!form.value.abilities.includes(ability)) {
        form.value.abilities.push(ability)
      }
    } else {
      form.value.abilities = form.value.abilities.filter((item) => {
        return item !== ability
      })
    }
  }
}

const onCheckItem = (group) => {
  for (const [ability] of Object.entries(props.abilities[group])) {
    if (!form.value.abilities.includes(ability)) {
      groupsChecked.value[group] = false
      return
    }
  }

  groupsChecked.value[group] = true
}

const emits = defineEmits(['generate'])

const onClickGenerate = () => {
  formRef.value.validate().then(() => {
    emits("generate")
  })
}
</script>