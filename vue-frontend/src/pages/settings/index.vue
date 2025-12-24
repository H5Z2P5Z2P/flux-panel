<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { NButton, NCard, NInput, NList, NListItem, NTag, NSpace, useMessage } from 'naive-ui';
import { 
  getPanelAddresses, 
  savePanelAddress, 
  setCurrentPanelAddress, 
  deletePanelAddress, 
  validatePanelAddress 
} from '@/utils/panel';
// We need to ensure reinitializeBaseURL is available or handled by window reload
import { reinitializeBaseURL } from '@/api/network'; // Ensure this exists or mock it

interface PanelAddress {
  name: string;
  address: string;
  inx: boolean;
}

const message = useMessage();
const addresses = ref<PanelAddress[]>([]);
const newName = ref('');
const newAddress = ref('');

// Helper since original code attached to window
const refreshList = () => {
    (window as any).setPanelAddresses = (list: PanelAddress[]) => {
        addresses.value = list;
    };
    getPanelAddresses();
};

const handleAdd = () => {
    if (!newName.value.trim() || !newAddress.value.trim()) {
        return message.error('请输入名称和地址');
    }
    if (!validatePanelAddress(newAddress.value.trim())) {
        return message.error('地址格式不正确，必须以 http:// 或 https:// 开头');
    }
    
    savePanelAddress(newName.value.trim(), newAddress.value.trim());
    message.success('添加成功');
    newName.value = '';
    newAddress.value = '';
    refreshList();
};

const handleSetCurrent = (name: string) => {
    setCurrentPanelAddress(name);
    refreshList();
    reinitializeBaseURL(); // This should update axios base url
    message.success('已切换面板地址');
    // window.location.reload(); // Optional if state needs full flush
};

const handleDelete = (name: string) => {
    deletePanelAddress(name);
    refreshList();
    reinitializeBaseURL();
    message.success('删除成功');
};

onMounted(() => {
    refreshList();
});
</script>

<template>
  <div class="p-6 max-w-4xl mx-auto">
      <h1 class="text-2xl font-bold mb-6">面板设置</h1>

      <div class="space-y-6">
          <NCard title="添加新面板地址">
              <div class="flex gap-4 items-end">
                  <div class="flex-1">
                      <div class="mb-1 text-sm text-gray-500">名称</div>
                      <NInput v-model:value="newName" placeholder="请输入名称" />
                  </div>
                  <div class="flex-[2]">
                      <div class="mb-1 text-sm text-gray-500">地址 (http://ip:port)</div>
                      <NInput v-model:value="newAddress" placeholder="http://127.0.0.1:8888" />
                  </div>
                  <NButton type="primary" @click="handleAdd">添加</NButton>
              </div>
          </NCard>

          <NCard title="已保存的面板地址">
              <NList>
                  <NListItem v-for="item in addresses" :key="item.name">
                      <div class="flex justify-between items-center">
                          <div>
                              <div class="flex items-center gap-2">
                                  <span class="font-bold">{{ item.name }}</span>
                                  <NTag v-if="item.inx" type="success" size="small">当前使用</NTag>
                              </div>
                              <div class="text-gray-500 text-sm mt-1">{{ item.address }}</div>
                          </div>
                          <div class="flex gap-2">
                              <NButton v-if="!item.inx" size="small" secondary type="primary" @click="handleSetCurrent(item.name)">
                                  设为当前
                              </NButton>
                              <NButton size="small" secondary type="error" @click="handleDelete(item.name)">
                                  删除
                              </NButton>
                          </div>
                      </div>
                  </NListItem>
                  <div v-if="addresses.length === 0" class="text-center text-gray-400 py-4">暂无数据</div>
              </NList>
          </NCard>
      </div>
  </div>
</template>
