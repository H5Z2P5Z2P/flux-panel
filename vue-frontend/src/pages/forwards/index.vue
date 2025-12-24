<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { NButton, NCard, NTag, useMessage, NModal, NForm, NFormItem, NInput, NSelect, NInputNumber, NEmpty, useDialog } from 'naive-ui';
import { getForwardList, createForward, updateForward, deleteForward, getTunnelList } from '@/api';
import { useForwardViewModel } from '@/composables/dashboard/useForwardViewModel';
import type { User, Tunnel } from '@/types';
import { isAdmin as checkIsAdmin } from '@/utils/auth';

// Reuse ForwardGroups logic if possible, but this is a management table/list.
// We can use the same Card style or a Table.
// Let's use Cards similar to Dashboard but with Edit/Delete buttons.

const forwards = ref<any[]>([]);
const tunnels = ref<Tunnel[]>([]);
const loading = ref(false);
const message = useMessage();
const dialog = useDialog();
const isAdmin = checkIsAdmin();

// Form
const showModal = ref(false);
const isEdit = ref(false);
const formModel = ref({
    id: 0,
    name: '',
    tunnelId: null as number | null,
    inPort: null as number | null,
    remoteAddr: '', // New line separated in UI, comma in API
    strategy: 'fifo',
    interfaceName: ''
});

const loadData = async () => {
    loading.value = true;
    try {
        const [fRes, tRes] = await Promise.all([getForwardList(), getTunnelList()]);
        if (fRes.code === 0) forwards.value = fRes.data || [];
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
        tunnelId: null,
        inPort: null,
        remoteAddr: '',
        strategy: 'fifo',
        interfaceName: ''
    };
    showModal.value = true;
};

const handleEditFunc = (item: any) => {
    isEdit.value = true;
    formModel.value = {
        id: item.id,
        name: item.name,
        tunnelId: item.tunnelId,
        inPort: item.inPort,
        remoteAddr: item.remoteAddr ? item.remoteAddr.replace(/,/g, '\n') : '',
        strategy: item.strategy || 'fifo',
        interfaceName: item.interfaceName || ''
    };
    showModal.value = true;
};

const handleDelete = (item: any) => {
    dialog.warning({
        title: '删除转发',
        content: `确定删除转发 ${item.name} 吗？`,
        positiveText: '确定',
        negativeText: '取消',
        onPositiveClick: async () => {
            const res = await deleteForward(item.id);
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
    const payload = {
        ...formModel.value,
        remoteAddr: formModel.value.remoteAddr.split('\n').map(s => s.trim()).filter(Boolean).join(',')
    };
    
    // Check multiple addresses for strategy
    const addrCount = payload.remoteAddr.split(',').length;
    if (addrCount <= 1) payload.strategy = 'fifo'; // default if single

    try {
        const res = isEdit.value ? await updateForward(payload) : await createForward(payload);
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

const getTunnelName = (id?: number) => tunnels.value.find(t => t.id === id)?.name || `ID:${id}`;

onMounted(() => {
    loadData();
});
</script>

<template>
  <div class="p-6">
    <div class="flex justify-between mb-6">
         <h1 class="text-xl font-bold">转发配置</h1>
         <NButton type="primary" @click="handleAdd">新增转发</NButton>
    </div>

    <div v-if="loading" class="text-center py-20"><div class="i-mdi-loading animate-spin text-2xl" /></div>
    <NEmpty v-else-if="forwards.length === 0" description="暂无转发规则" class="py-20" />

    <div v-else class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
        <NCard v-for="item in forwards" :key="item.id" size="small" hoverable>
            <template #header>
                <div class="flex justify-between items-center">
                    <span class="font-bold border-l-4 border-purple-500 pl-2">{{ item.name }}</span>
                    <NTag size="small" type="success" v-if="item.serviceRunning">运行中</NTag>
                    <NTag size="small" type="error" v-else>已停止</NTag>
                </div>
            </template>
            
            <div class="space-y-3 text-sm">
                 <div class="flex justify-between border-b pb-2 border-dashed border-gray-200 dark:border-gray-700">
                    <span class="text-gray-500">隧道</span>
                    <span class="font-mono">{{ item.tunnelName || getTunnelName(item.tunnelId) }}</span>
                </div>
                <div class="flex justify-between border-b pb-2 border-dashed border-gray-200 dark:border-gray-700">
                    <span class="text-gray-500">入口</span>
                    <span class="font-mono text-xs">
                        {{ item.inIp }}:{{ item.inPort }}
                    </span>
                </div>
                <div>
                    <span class="text-gray-500 block mb-1">目标地址</span>
                    <div class="bg-gray-50 dark:bg-gray-800 p-2 rounded text-xs font-mono break-all max-h-20 overflow-y-auto">
                        {{ item.remoteAddr }}
                    </div>
                </div>
                 
                <div class="grid grid-cols-3 gap-1 text-xs text-center pt-2">
                    <div>
                        <div class="text-green-600 font-bold">{{ item.inFlow || 0 }} B</div>
                        <div class="text-gray-400">上传</div>
                    </div>
                    <div>
                         <div class="text-orange-600 font-bold">{{ item.outFlow || 0 }} B</div>
                         <div class="text-gray-400">下载</div>
                    </div>
                     <div>
                         <div class="text-purple-600 font-bold">{{ (item.inFlow||0)+(item.outFlow||0) }} B</div>
                         <div class="text-gray-400">总计</div>
                    </div>
                </div>
            </div>

            <template #action>
                <div class="flex justify-end gap-2">
                    <NButton size="tiny" secondary type="primary" @click="handleEditFunc(item)">编辑</NButton>
                    <NButton size="tiny" secondary type="error" @click="handleDelete(item)">删除</NButton>
                </div>
            </template>
        </NCard>
    </div>

    <!-- Modal -->
    <NModal v-model:show="showModal" preset="card" :title="isEdit ? '编辑转发' : '新增转发'" style="width: 600px">
        <NForm label-placement="left" label-width="100">
            <NFormItem label="名称"><NInput v-model:value="formModel.name" /></NFormItem>
            <NFormItem label="隧道">
                <NSelect v-model:value="formModel.tunnelId" :options="tunnels.map(t => ({ label: t.name, value: t.id }))" filterable />
            </NFormItem>
            <NFormItem label="端口/InPort" v-if="!formModel.inPort || isEdit"> <!-- Some tunnels might auto assign? Usually manual -->
                 <NInputNumber v-model:value="formModel.inPort" :min="1" :max="65535" placeholder="留空则随机?" class="w-full" />
            </NFormItem>
            <NFormItem label="目标地址">
                <NInput v-model:value="formModel.remoteAddr" type="textarea" placeholder="IP:Port，多行支持多个" />
            </NFormItem>
             <NFormItem label="负载策略" v-if="(formModel.remoteAddr.split('\n').filter(Boolean).length > 1)">
                <NSelect v-model:value="formModel.strategy" :options="[{label: '轮询', value: 'rr'}, {label: '顺序', value: 'fifo'}, {label: '随机', value: 'random'}]" />
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
