<script setup lang="ts">
import { ref, onMounted, reactive, computed } from 'vue';
import { NCard, NForm, NFormItem, NInput, NSwitch, NSelect, NButton, useMessage, NDivider, NAlert } from 'naive-ui';
import { useConfigStore } from '@/stores/config';
import { getConfigs, updateConfigs } from '@/api';
// We might need to implement getConfigs/updateConfigs in api/index.ts if not present

const message = useMessage();
const configStore = useConfigStore();
const loading = ref(false);
const saving = ref(false);
const hasChanges = ref(false);

const configs = reactive<Record<string, string>>({});
const originalConfigs = reactive<Record<string, string>>({});

const configItems = [
  {
    key: 'ip',
    label: '面板后端地址',
    placeholder: '请输入面板后端IP:PORT',
    description: '格式“ip:port”,用于对接节点时使用,ip是你安装面板服务器的公网ip,端口是安装脚本内输入的后端端口。不要套CDN,不支持https,通讯数据有加密',
    type: 'input'
  },
  {
    key: 'app_name',
    label: '应用名称',
    placeholder: '请输入应用名称',
    description: '在浏览器标签页和导航栏显示的应用名称',
    type: 'input'
  },
  {
    key: 'captcha_enabled',
    label: '启用验证码',
    description: '开启后，用户登录时需要完成验证码验证',
    type: 'switch'
  },
  {
    key: 'captcha_type',
    label: '验证码类型',
    description: '选择验证码的显示类型，不同类型有不同的安全级别',
    type: 'select',
    dependsOn: 'captcha_enabled',
    dependsValue: 'true',
    options: [
      { label: '随机类型', value: 'RANDOM', description: '系统随机选择验证码类型' },
      { label: '滑块验证码', value: 'SLIDER', description: '拖动滑块完成拼图验证' },
      { label: '文字点选验证码', value: 'WORD_IMAGE_CLICK', description: '按顺序点击指定文字' },
      { label: '旋转验证码', value: 'ROTATE', description: '旋转图片到正确角度' },
      { label: '拼图验证码', value: 'CONCAT', description: '拖动滑块完成图片拼接' }
    ]
  }
];

const loadData = async () => {
    loading.value = true;
    try {
        const res = await getConfigs();
        if (res.code === 0 && res.data) {
            Object.assign(configs, res.data);
            Object.assign(originalConfigs, res.data);
            hasChanges.value = false;
        } else {
            // fallback or empty
        }
    } catch (e) {
        message.error('加载配置失败');
    } finally {
        loading.value = false;
    }
};

const handleSave = async () => {
    saving.value = true;
    try {
        const res = await updateConfigs(configs);
        if (res.code === 0) {
            message.success('配置保存成功');
            Object.assign(originalConfigs, configs);
            hasChanges.value = false;
            // Update store if needed for global app name change
            if (configs.app_name) {
                document.title = configs.app_name;
                // configStore.appName = configs.app_name; // if store has it
            }
        } else {
            message.error(res.msg || '保存失败');
        }
    } catch (e) {
        message.error('保存失败');
    } finally {
        saving.value = false;
    }
};

const handleChange = (key: string, val: any) => {
    // Logic for depends
    if (key === 'captcha_enabled' && val === 'true') {
        if (!configs.captcha_type) configs.captcha_type = 'RANDOM';
    }
    
    // Check changes
    let changed = false;
    for (const k in configs) {
        if (configs[k] !== originalConfigs[k]) {
            changed = true;
            break;
        }
    }
    hasChanges.value = changed;
};

// Helper for conditional render
const shouldShow = (item: any) => {
    if (!item.dependsOn) return true;
    return configs[item.dependsOn] === item.dependsValue;
};

onMounted(() => {
    loadData();
});
</script>

<template>
  <div class="p-6 max-w-4xl mx-auto">
      <div class="flex items-center justify-between mb-6">
          <h1 class="text-2xl font-bold">网站配置</h1>
          <NButton type="primary" :loading="saving" :disabled="!hasChanges" @click="handleSave">
              保存配置
          </NButton>
      </div>

      <NCard v-if="loading" class="text-center py-20">
          <div class="i-mdi-loading animate-spin text-2xl" />
      </NCard>
      
      <NCard v-else class="shadow-sm">
          <NForm label-placement="top">
              <template v-for="item in configItems" :key="item.key">
                  <NFormItem v-if="shouldShow(item)" :label="item.label">
                      <div class="w-full space-y-1">
                          <p v-if="item.description" class="text-xs text-gray-500 mb-2">{{ item.description }}</p>
                          
                          <NInput 
                             v-if="item.type === 'input'" 
                             v-model:value="configs[item.key]" 
                             :placeholder="item.placeholder"
                             @update:value="handleChange(item.key, $event)"
                          />

                          <NSwitch 
                             v-if="item.type === 'switch'"
                             :value="configs[item.key] === 'true'"
                             @update:value="(v) => { configs[item.key] = String(v); handleChange(item.key, String(v)); }"
                          >
                             <template #checked>已启用</template>
                             <template #unchecked>已禁用</template>
                          </NSwitch>

                          <NSelect
                             v-if="item.type === 'select'"
                             v-model:value="configs[item.key]"
                             :options="item.options"
                             @update:value="handleChange(item.key, $event)"
                          />
                      </div>
                  </NFormItem>
                  <NDivider v-if="shouldShow(item)" />
              </template>
          </NForm>
      </NCard>

       <NAlert v-if="hasChanges" type="warning" class="mt-4 fixed bottom-8 right-8 shadow-lg z-50">
           配置已修改，请记得保存
       </NAlert>
  </div>
</template>
