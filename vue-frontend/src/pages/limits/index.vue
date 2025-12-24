<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { NButton, NCard, NTag, NInput, NSelect, NModal, NForm, NFormItem, NInputNumber, useMessage, useDialog, NEmpty } from 'naive-ui';
import { getSpeedLimitList, createSpeedLimit, updateSpeedLimit, deleteSpeedLimit, getTunnelList } from '@/api';

interface SpeedLimitRule {
  id: number;
  name: string;
  speed: number;
  status: number;
  tunnelId: number;
  tunnelName: string;
}

const rules = ref<SpeedLimitRule[]>([]);
const tunnels = ref<any[]>([]);
const loading = ref(false);
const message = useMessage();
const dialog = useDialog();

// Form
const showModal = ref(false);
const isEdit = ref(false);
const formModel = ref({
    id: 0,
    name: '',
    speed: 100,
    tunnelId: null as number | null,
    status: 1
});

const loadData = async () => {
    loading.value = true;
    try {
        const [rRes, tRes] = await Promise.all([getSpeedLimitList(), getTunnelList()]);
        if (rRes.code === 0) rules.value = rRes.data || [];
        if (tRes.code === 0) tunnels.value = tRes.data || [];
    } catch (e) {
        message.error('加载数据失败');
    } finally {
        loading.value = false;
    }
};

const handleAdd = () => {
    isEdit.value = false;
    formModel.value = {
        id: 0,
        name: '',
        speed: 100,
        tunnelId: null,
        status: 1
    };
    showModal.value = true;
};

const handleEditFunc = (rule: SpeedLimitRule) => {
    isEdit.value = true;
    formModel.value = {
        id: rule.id,
        name: rule.name,
        speed: rule.speed,
        tunnelId: rule.tunnelId,
        status: rule.status
    };
    showModal.value = true;
};

const handleDelete = (rule: SpeedLimitRule) => {
    dialog.warning({
        title: '删除规则',
        content: `确定删除限速规则 ${rule.name} 吗？`,
        positiveText: '确定',
        negativeText: '取消',
        onPositiveClick: async () => {
             const res = await deleteSpeedLimit(rule.id);
             if (res.code === 0) {
                 message.success('删除成功');
                 loadData();
             } else {
                 message.error(res.msg);
             }
        }
    });
};

const submitForm = async () => {
    try {
        const payload = { ...formModel.value };
        if (!payload.tunnelId) return message.warning('请选择隧道');
        
        const res = isEdit.value ? await updateSpeedLimit(payload) : await createSpeedLimit(payload);
        if (res.code === 0) {
            message.success(isEdit.value ? '更新成功' : '创建成功');
            showModal.value = false;
            loadData();
        } else {
            message.error(res.msg);
        }
    } catch (e) {
        message.error('提交失败');
    }
};

const getTunnelName = (id: number) => tunnels.value.find(t => t.id === id)?.name || `ID:${id}`;

onMounted(() => {
    loadData();
});
</script>

<template>
  <div class="p-6">
     <div class="flex justify-between mb-6">
         <h1 class="text-xl font-bold">限速管理</h1>
         <NButton type="primary" @click="handleAdd">新增规则</NButton>
     </div>

     <div v-if="loading" class="text-center py-20"><div class="i-mdi-loading animate-spin text-2xl" /></div>
     <NEmpty v-else-if="rules.length === 0" description="暂无限速规则" class="py-20" />
     
     <div v-else class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
         <NCard v-for="rule in rules" :key="rule.id" size="small" hoverable>
             <template #header>
                 <div class="flex justify-between items-center">
                     <span class="font-bold">{{ rule.name }}</span>
                     <NTag size="small" :type="rule.status === 1 ? 'success' : 'error'">
                         {{ rule.status === 1 ? '运行' : '异常' }}
                     </NTag>
                 </div>
             </template>
             
             <div class="space-y-3 py-2">
                 <div class="flex justify-between text-sm">
                     <span class="text-gray-500">速度限制</span>
                     <NTag size="small" type="info">{{ rule.speed }} Mbps</NTag>
                 </div>
                 <div class="flex justify-between text-sm">
                     <span class="text-gray-500">绑定隧道</span>
                     <NTag size="small">{{ rule.tunnelName || getTunnelName(rule.tunnelId) }}</NTag>
                 </div>
             </div>

             <template #action>
                 <div class="flex justify-end gap-2">
                     <NButton size="tiny" secondary type="primary" @click="handleEditFunc(rule)">编辑</NButton>
                     <NButton size="tiny" secondary type="error" @click="handleDelete(rule)">删除</NButton>
                 </div>
             </template>
         </NCard>
     </div>

     <NModal v-model:show="showModal" preset="card" :title="isEdit ? '编辑规则' : '新增规则'" style="width: 500px">
         <NForm label-placement="left" label-width="80">
             <NFormItem label="名称"><NInput v-model:value="formModel.name" /></NFormItem>
             <NFormItem label="限速(Mbps)">
                 <NInputNumber v-model:value="formModel.speed" :min="1" class="w-full" />
             </NFormItem>
             <NFormItem label="绑定隧道">
                 <NSelect v-model:value="formModel.tunnelId" :options="tunnels.map(t => ({label: t.name, value: t.id}))" filterable />
             </NFormItem>
         </NForm>
         <template #footer>
             <div class="flex justify-end gap-2">
                 <NButton @click="showModal = false">取消</NButton>
                 <NButton type="primary" @click="submitForm">提交</NButton>
             </div>
         </template>
     </NModal>
  </div>
</template>
