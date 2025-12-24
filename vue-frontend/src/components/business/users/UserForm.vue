<script setup lang="ts">
import { ref, watch } from 'vue';
import { NForm, NFormItem, NInput, NInputNumber, NSelect, NDatePicker, NSwitch, NButton } from 'naive-ui';
import type { FormRules } from 'naive-ui';
import type { UserForm } from '@/types';

const props = defineProps<{
  modelValue: UserForm;
  isEdit?: boolean;
}>();

const emit = defineEmits(['update:modelValue', 'submit']);

const formRef = ref<any>(null);
const localForm = ref<UserForm>({ ...props.modelValue });

watch(() => props.modelValue, (val) => {
  localForm.value = { ...val };
}, { deep: true });

watch(localForm, (val) => {
  emit('update:modelValue', val);
}, { deep: true });

const rules: FormRules = {
  user: { required: true, message: '请输入用户名', trigger: 'blur' },
  status: { type: 'number', required: true, message: '请选择状态', trigger: 'change' },
  flow: { type: 'number', required: true, message: '请输入流量限制', trigger: 'blur' },
  // expTime: { type: 'date', required: true, message: '请选择过期时间', trigger: 'change' } // NDatePicker returns timestamp or Date depending
};

const handleSubmit = (e: MouseEvent) => {
  e.preventDefault();
  formRef.value?.validate((errors: any) => {
    if (!errors) {
      emit('submit');
    }
  });
};
</script>

<template>
  <NForm ref="formRef" :model="localForm" :rules="rules" label-placement="left" label-width="100" require-mark-placement="right-hanging">
    <NFormItem label="用户名" path="user">
      <NInput v-model:value="localForm.user" placeholder="请输入用户名" :disabled="isEdit" />
    </NFormItem>
    
    <NFormItem label="显示名称" path="name">
      <NInput v-model:value="localForm.name" placeholder="请输入显示名称" />
    </NFormItem>

    <NFormItem label="密码" path="pwd">
      <NInput 
         v-model:value="localForm.pwd" 
         type="password" 
         show-password-on="click"
         :placeholder="isEdit ? '不修改请留空' : '请输入密码'" 
      />
    </NFormItem>

    <NFormItem label="状态" path="status">
      <NSelect v-model:value="localForm.status" :options="[{label: '正常', value: 1}, {label: '禁用', value: 0}]" />
    </NFormItem>

    <NFormItem label="流量限制 (GB)" path="flow">
      <NInputNumber v-model:value="localForm.flow" :min="0" />
    </NFormItem>

    <NFormItem label="转发数量" path="num">
      <NInputNumber v-model:value="localForm.num" :min="0" />
    </NFormItem>

    <NFormItem label="过期时间" path="expTime">
       <NDatePicker v-model:value="localForm.expTime" type="datetime" clearable />
    </NFormItem>

    <NFormItem label="流量重置日" path="flowResetTime">
       <NInputNumber v-model:value="localForm.flowResetTime" :min="0" :max="31" placeholder="0为不重置" />
    </NFormItem>

    <div class="flex justify-end pt-4">
       <NButton type="primary" attr-type="button" @click="handleSubmit">保存</NButton>
    </div>
  </NForm>
</template>
