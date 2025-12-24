<script setup lang="ts">
import { ref, reactive } from 'vue';
import { NCard, NForm, NFormItem, NInput, NButton, useMessage } from 'naive-ui';
import { updatePassword } from '@/api';
import { useRouter } from 'vue-router';

const router = useRouter();
const message = useMessage();
const loading = ref(false);

const formModel = reactive({
    oldPassword: '',
    newPassword: '',
    confirmPassword: ''
});

const rules = {
    oldPassword: { required: true, message: '请输入旧密码', trigger: 'blur' },
    newPassword: { required: true, message: '请输入新密码', trigger: 'blur' },
    confirmPassword: { 
        required: true, 
        validator: (_: any, value: string) => {
             if (!value) return new Error('请确认密码');
             if (value !== formModel.newPassword) return new Error('两次输入密码不一致');
             return true;
        }, 
        trigger: 'blur' 
    }
};

const formRef = ref<any>(null);

const handleSubmit = (e: MouseEvent) => {
    e.preventDefault();
    formRef.value?.validate(async (errors: any) => {
        if (!errors) {
            loading.value = true;
            try {
                const res = await updatePassword({
                    oldPassword: formModel.oldPassword,
                    newPassword: formModel.newPassword
                });

                if (res.code === 0) {
                    message.success('密码修改成功，请重新登录');
                    localStorage.clear();
                    router.push('/login');
                } else {
                    message.error(res.msg || '修改失败');
                }
            } catch (err) {
                message.error('请求失败');
            } finally {
                loading.value = false;
            }
        }
    });
};
</script>

<template>
  <div class="max-w-md mx-auto py-10 px-4">
      <NCard title="修改密码" class="shadow-sm">
          <NForm ref="formRef" :model="formModel" :rules="rules" label-placement="top">
              <NFormItem label="旧密码" path="oldPassword">
                  <NInput v-model:value="formModel.oldPassword" type="password" placeholder="请输入旧密码" show-password-on="click"/>
              </NFormItem>
              <NFormItem label="新密码" path="newPassword">
                  <NInput v-model:value="formModel.newPassword" type="password" placeholder="请输入新密码" show-password-on="click"/>
              </NFormItem>
               <NFormItem label="确认新密码" path="confirmPassword">
                  <NInput v-model:value="formModel.confirmPassword" type="password" placeholder="请再次输入新密码" show-password-on="click"/>
              </NFormItem>
              
              <div class="pt-4">
                  <NButton type="primary" block :loading="loading" @click="handleSubmit">
                      确认修改
                  </NButton>
              </div>
          </NForm>
      </NCard>
  </div>
</template>
