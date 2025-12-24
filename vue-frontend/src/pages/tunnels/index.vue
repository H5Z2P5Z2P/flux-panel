<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { NButton, NCard, NTag, useMessage, NModal, NForm, NFormItem, NInput, NSelect, NInputNumber, NEmpty, useDialog } from 'naive-ui';
import { getTunnelList, createTunnel, updateTunnel, deleteTunnel, getNodeList, diagnoseTunnel } from '@/api';
import type { Tunnel } from '@/types';

const tunnels = ref<Tunnel[]>([]);
const nodes = ref<any[]>([]); // simplified node type
const loading = ref(false);
const message = useMessage();
const dialog = useDialog();

// Form
const showModal = ref(false);
const isEdit = ref(false);
const showDiagnoseModal = ref(false);
const diagnosing = ref(false);
const diagnoseResult = ref<any>(null);

const formModel = ref({
    id: 0,
    name: '',
    type: 1, // 1: forwarding, 2: tunnel
    inNodeId: null as number | null,
    outNodeId: null as number | null,
    protocol: 'tls',
    tcpListenAddr: '[::]',
    udpListenAddr: '[::]',
    interfaceName: '',
    flow: 1, // 1: single, 2: double
    trafficRatio: 1.0,
    status: 1
});

const loadData = async () => {
    loading.value = true;
    try {
        const [tRes, nRes] = await Promise.all([getTunnelList(), getNodeList()]);
        if (tRes.code === 0) tunnels.value = tRes.data || [];
        if (nRes.code === 0) nodes.value = nRes.data || [];
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
        type: 1,
        inNodeId: null,
        outNodeId: null,
        protocol: 'tls',
        tcpListenAddr: '[::]',
        udpListenAddr: '[::]',
        interfaceName: '',
        flow: 1,
        trafficRatio: 1.0,
        status: 1
    };
    showModal.value = true;
};

const handleEditFunc = (t: Tunnel) => {
    isEdit.value = true;
    formModel.value = {
        id: t.id,
        name: t.name,
        type: t.type || 1,
        inNodeId: t.inNodeId || null,
        outNodeId: t.outNodeId || null,
        protocol: t.protocol || 'tls',
        tcpListenAddr: t.tcpListenAddr || '[::]',
        udpListenAddr: t.udpListenAddr || '[::]',
        interfaceName: t.interfaceName || '',
        flow: t.flow || 1,
        trafficRatio: t.trafficRatio || 1.0,
        status: t.status || 1
    } as any;
    showModal.value = true;
};

const handleDelete = (t: Tunnel) => {
    dialog.warning({
        title: '删除隧道',
        content: `确定删除隧道 ${t.name} 吗？`,
        positiveText: '确定',
        negativeText: '取消',
        onPositiveClick: async () => {
            const res = await deleteTunnel(t.id);
            if (res.code === 0) {
                message.success('删除成功');
                loadData();
            } else {
                message.error(res.msg);
            }
        }
    });
};

const handleDiagnose = async (t: Tunnel) => {
    showDiagnoseModal.value = true;
    diagnosing.value = true;
    diagnoseResult.value = null;
    try {
        const res = await diagnoseTunnel(t.id);
        if (res.code === 0) {
            diagnoseResult.value = res.data;
        } else {
            message.error(res.msg || '诊断失败');
        }
    } catch (e) {
        message.error('网络错误');
    } finally {
        diagnosing.value = false;
    }
};

const submitForm = async () => {
    try {
        const res = isEdit.value ? await updateTunnel(formModel.value) : await createTunnel(formModel.value);
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

const getNodeName = (id?: number) => nodes.value.find(n => n.id === id)?.name || `ID:${id}`;

onMounted(() => {
    loadData();
});
</script>

<template>
  <div class="p-6">
    <div class="flex justify-between mb-6">
         <h1 class="text-xl font-bold">隧道管理</h1>
         <NButton type="primary" @click="handleAdd">新增隧道</NButton>
    </div>

    <div v-if="loading" class="text-center py-20"><div class="i-mdi-loading animate-spin text-2xl" /></div>
    <NEmpty v-else-if="tunnels.length === 0" description="暂无隧道" class="py-20" />

    <div v-else class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
        <NCard v-for="tunnel in tunnels" :key="tunnel.id" size="small" hoverable>
            <template #header>
                <div class="flex justify-between items-center">
                    <span class="font-bold border-l-4 border-secondary pl-2">{{ tunnel.name }}</span>
                    <div class="flex gap-2">
                        <NTag size="small" :type="tunnel.type === 1 ? 'info' : 'warning'">
                            {{ tunnel.type === 1 ? '端口转发' : '隧道转发' }}
                        </NTag>
                        <NTag size="small" :type="tunnel.status === 1 ? 'success' : 'default'">
                            {{ tunnel.status === 1 ? '启用' : '禁用' }}
                        </NTag>
                    </div>
                </div>
            </template>
            
            <div class="space-y-3 text-sm">
                <div class="flex justify-between border-b pb-2 border-dashed border-gray-200 dark:border-gray-700">
                    <span class="text-gray-500">入口节点</span>
                    <span class="font-mono">{{ getNodeName(tunnel.inNodeId) }}</span>
                </div>
                <div class="flex justify-between border-b pb-2 border-dashed border-gray-200 dark:border-gray-700">
                    <span class="text-gray-500">出口节点</span>
                    <span class="font-mono">
                        {{ tunnel.type === 1 ? '同入口' : getNodeName(tunnel.outNodeId) }}
                    </span>
                </div>
                <div class="grid grid-cols-2 gap-2 text-xs text-gray-400">
                    <div>
                        <div>流量计算: {{ tunnel.flow === 1 ? '单向' : '双向' }}</div>
                        <div>倍率: x{{ tunnel.trafficRatio }}</div>
                    </div>
                </div>
            </div>

            <template #action>
                <div class="flex justify-end gap-2">
                    <NButton size="tiny" secondary type="warning" @click="handleDiagnose(tunnel)">诊断</NButton>
                    <NButton size="tiny" secondary type="primary" @click="handleEditFunc(tunnel)">编辑</NButton>
                    <NButton size="tiny" secondary type="error" @click="handleDelete(tunnel)">删除</NButton>
                </div>
            </template>
        </NCard>
    </div>

     <NModal v-model:show="showModal" preset="card" :title="isEdit ? '编辑隧道' : '新增隧道'" style="width: 700px">
         <NForm label-placement="left" label-width="120">
             <NFormItem label="隧道名称"><NInput v-model:value="formModel.name" /></NFormItem>
             <NFormItem label="类型">
                 <NSelect v-model:value="formModel.type" :options="[{label: '端口转发', value: 1}, {label: '隧道转发', value: 2}]" />
             </NFormItem>
             
             <NFormItem label="入口节点">
                 <NSelect v-model:value="formModel.inNodeId" :options="nodes.map(n => ({label: n.name, value: n.id}))" filterable />
             </NFormItem>

             <NFormItem label="出口节点" v-if="formModel.type === 2">
                 <NSelect v-model:value="formModel.outNodeId" :options="nodes.map(n => ({label: n.name, value: n.id}))" filterable />
             </NFormItem>

             <div class="grid grid-cols-2 gap-4">
                 <NFormItem label="流量计算">
                     <NSelect v-model:value="formModel.flow" :options="[{label: '单向', value: 1}, {label: '双向', value: 2}]" />
                 </NFormItem>
                 <NFormItem label="倍率">
                     <NInputNumber v-model:value="formModel.trafficRatio" :step="0.1" />
                 </NFormItem>
             </div>
             
             <div class="grid grid-cols-2 gap-4">
                 <NFormItem label="TCP监听"><NInput v-model:value="formModel.tcpListenAddr" /></NFormItem>
                 <NFormItem label="UDP监听"><NInput v-model:value="formModel.udpListenAddr" /></NFormItem>
             </div>
         </NForm>
         <template #footer>
             <div class="flex justify-end gap-2">
                 <NButton @click="showModal = false">取消</NButton>
                 <NButton type="primary" @click="submitForm">提交</NButton>
             </div>
         </template>
     </NModal>

     <NModal v-model:show="showDiagnoseModal" preset="card" title="诊断结果" style="width: 600px">
         <div v-if="diagnosing" class="text-center py-10">诊断中...</div>
         <div v-else-if="diagnoseResult">
             <!-- Simple result display -->
             <div v-for="(res, idx) in diagnoseResult.results" :key="idx" class="mb-4 p-3 rounded" :class="res.success ? 'bg-green-50 dark:bg-green-900/20' : 'bg-red-50 dark:bg-red-900/20'">
                 <div class="font-bold" :class="res.success ? 'text-green-600' : 'text-red-600'">{{ res.success ? '通过' : '失败' }}</div>
                 <div class="text-sm mt-1">
                     <div>节点: {{ res.nodeName }}</div>
                     <div>目标: {{ res.targetIp }}:{{ res.targetPort }}</div>
                     <div v-if="res.message">{{ res.message }}</div>
                     <div v-if="res.averageTime">延迟: {{ res.averageTime }}ms</div>
                 </div>
             </div>
         </div>
     </NModal>
  </div>
</template>
