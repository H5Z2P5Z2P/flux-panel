<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import { NButton, NCard, NTag, NProgress, useMessage, NGrid, NGridItem, NEmpty, NModal, NForm, NFormItem, NInput, NInputNumber, NSwitch, useDialog } from 'naive-ui';
import { getNodeList, createNode, updateNode, deleteNode, getNodeInstallCommand } from '@/api';
import { useClipboard } from '@vueuse/core';
import { isWebViewFunc } from '@/utils/panel';

// Interfaces (Should be in types/index.ts but simplified here for migration speed if not present)
// Actually types/index.ts doesn't have Node interface fully defined with systemInfo.
// I will define local interface or extend.
interface NodeSystemInfo {
  cpuUsage: number;
  memoryUsage: number;
  uploadTraffic: number;
  downloadTraffic: number;
  uploadSpeed: number;
  downloadSpeed: number;
  uptime: number;
}

interface NodeItem {
  id: number;
  name: string;
  ip: string;
  serverIp: string;
  portSta: number;
  portEnd: number;
  status: number;
  connectionStatus: 'online' | 'offline';
  systemInfo?: NodeSystemInfo;
  version?: string;
}

const nodes = ref<NodeItem[]>([]);
const loading = ref(false);
const message = useMessage();
const dialog = useDialog();
const { copy, isSupported } = useClipboard();

// WebSocket logic
const ws = ref<WebSocket | null>(null);

// Form
const showModal = ref(false);
const isEdit = ref(false);
const formModel = ref({
    id: 0,
    name: '',
    ipString: '', // multiline
    serverIp: '',
    portSta: 1000,
    portEnd: 65535,
    http: 0,
    tls: 0,
    socks: 0
});

const loadNodes = async () => {
    loading.value = true;
    try {
        const res = await getNodeList();
        if (res.code === 0) {
            nodes.value = res.data.map((n: any) => ({
                ...n,
                connectionStatus: n.status === 1 ? 'online' : 'offline',
                systemInfo: undefined
            }));
        }
    } catch (e) {
        message.error('加载节点列表失败');
    } finally {
        loading.value = false;
    }
};

const initWebSocket = () => {
    const baseUrl = import.meta.env.VITE_API_BASE || '/api/v1/';
    // Handle relative path or absolute URL
    let wsUrl = '';
    
    // Safety check for absolute http/https
    if (baseUrl.startsWith('http')) {
        wsUrl = baseUrl.replace(/^http/, 'ws').replace(/\/api\/v1\/?$/, '') + `/system-info?type=0&secret=${localStorage.getItem('token')}`;
    } else {
        // relative path, construct absolute ws url
        // if baseUrl is '/api/v1/', result should be ws://host/api/v1/...
        wsUrl = `ws://${window.location.host}${baseUrl.startsWith('/') ? '' : '/'}${baseUrl.replace(/\/api\/v1\/?$/, '')}/system-info?type=0&secret=${localStorage.getItem('token')}`;
    }

    ws.value = new WebSocket(wsUrl);
    ws.value.onmessage = (event) => {
        try {
            const data = JSON.parse(event.data);
            handleWsMessage(data);
        } catch (e) {}
    };
    // Reconnect logic omitted for brevity, simpler restart on page reload
};

const handleWsMessage = (data: any) => {
    const { id, type, data: msgData } = data;
    const nodeIndex = nodes.value.findIndex(n => n.id == id);
    if (nodeIndex === -1 || !nodes.value[nodeIndex]) return;

    const node = nodes.value[nodeIndex];

    if (type === 'status') {
        node.connectionStatus = msgData === 1 ? 'online' : 'offline';
        if (msgData === 0) node.systemInfo = undefined;
    } else if (type === 'info') {
        let info = typeof msgData === 'string' ? JSON.parse(msgData) : msgData;
        // Simplified mapping, assuming backend consistent
        node.systemInfo = {
            cpuUsage: parseFloat(info.cpu_usage) || 0,
            memoryUsage: parseFloat(info.memory_usage) || 0,
            uploadTraffic: parseInt(info.bytes_transmitted) || 0,
            downloadTraffic: parseInt(info.bytes_received) || 0,
            // Speed calculation requires prev state, skipped for now to keep it simple or adds complexity
            uploadSpeed: 0, 
            downloadSpeed: 0,
            uptime: parseInt(info.uptime) || 0
        };
        node.connectionStatus = 'online';
    }
};

const handleInstall = async (node: NodeItem) => {
    try {
        const res = await getNodeInstallCommand(node.id);
        if (res.code === 0 && res.data) {
            if (isSupported) {
                await copy(res.data);
                message.success('安装命令已复制');
            } else {
                // Show modal with command
                dialog.info({
                    title: '安装命令',
                    content: res.data,
                    positiveText: '关闭'
                });
            }
        }
    } catch (e) {
        message.error('获取安装命令失败');
    }
};

const handleDelete = (node: NodeItem) => {
    dialog.warning({
        title: '删除节点',
        content: `确定删除节点 ${node.name} 吗？`,
        positiveText: '确定',
        negativeText: '取消',
        onPositiveClick: async () => {
            const res = await deleteNode(node.id);
            if (res.code === 0) {
                message.success('删除成功');
                loadNodes();
            } else {
                message.error(res.msg);
            }
        }
    });
};

const handleAdd = () => {
    isEdit.value = false;
    formModel.value = {
        id: 0,
        name: '',
        ipString: '', 
        serverIp: '',
        portSta: 1000,
        portEnd: 65535,
        http: 0,
        tls: 0,
        socks: 0
    };
    showModal.value = true;
};

const handleEditFunc = (node: NodeItem) => {
    isEdit.value = true;
    formModel.value = {
        id: node.id,
        name: node.name,
        ipString: node.ip ? node.ip.split(',').join('\n') : '',
        serverIp: node.serverIp,
        portSta: node.portSta,
        portEnd: node.portEnd,
        http: (node as any).http || 0,
        tls: (node as any).tls || 0,
        socks: (node as any).socks || 0
    };
    showModal.value = true;
};

const submitForm = async () => {
    const payload = {
        ...formModel.value,
        ip: formModel.value.ipString.split('\n').map(s => s.trim()).filter(Boolean).join(',')
    };
    // remove ipString from payload handled by API? axios sends what we accept. formModel has it but we override 'ip' key.
    
    try {
        const res = isEdit.value ? await updateNode(payload) : await createNode(payload);
        if (res.code === 0) {
            message.success(isEdit.value ? '更新成功' : '创建成功');
            showModal.value = false;
            loadNodes();
        } else {
             message.error(res.msg);
        }
    } catch (e) {
        message.error('提交失败');
    }
};

onMounted(() => {
    loadNodes();
    initWebSocket();
});

onUnmounted(() => {
    if (ws.value) ws.value.close();
});

// Formatters
const formatBytes = (bytes?: number) => {
    if (!bytes) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};
</script>

<template>
  <div class="p-6">
     <div class="flex justify-between mb-6">
         <h1 class="text-xl font-bold">节点管理</h1>
         <NButton type="primary" @click="handleAdd">新增节点</NButton>
     </div>

     <div v-if="loading" class="text-center py-20"><div class="i-mdi-loading animate-spin text-2xl" /></div>
     <NEmpty v-else-if="nodes.length === 0" description="暂无节点" class="py-20" />
     
     <div v-else class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
         <NCard v-for="node in nodes" :key="node.id" size="small" hoverable>
             <template #header>
                 <div class="flex justify-between items-center">
                     <span class="font-bold border-l-4 border-primary pl-2">{{ node.name }}</span>
                     <NTag size="small" :type="node.connectionStatus === 'online' ? 'success' : 'error'">
                         {{ node.connectionStatus === 'online' ? '在线' : '离线' }}
                     </NTag>
                 </div>
             </template>
             <template #header-extra>
                 <div class="text-xs text-gray-400 font-mono">{{ node.serverIp }}</div>
             </template>

             <div class="space-y-4">
                 <!-- System Info -->
                 <div class="grid grid-cols-2 gap-4">
                     <div>
                         <div class="flex justify-between text-xs mb-1">
                             <span>CPU</span>
                             <span>{{ node.systemInfo?.cpuUsage.toFixed(1) || 0 }}%</span>
                         </div>
                         <NProgress :percentage="node.systemInfo?.cpuUsage || 0" :show-indicator="false" :height="4" 
                            :status="(node.systemInfo?.cpuUsage||0) > 80 ? 'error' : 'success'"
                         />
                     </div>
                     <div>
                         <div class="flex justify-between text-xs mb-1">
                             <span>RAM</span>
                             <span>{{ node.systemInfo?.memoryUsage.toFixed(1) || 0 }}%</span>
                         </div>
                         <NProgress :percentage="node.systemInfo?.memoryUsage || 0" :show-indicator="false" :height="4"
                             :status="(node.systemInfo?.memoryUsage||0) > 80 ? 'error' : 'success'"
                         />
                     </div>
                 </div>

                 <!-- Network Info -->
                 <div class="grid grid-cols-2 gap-2 text-xs">
                     <div class="bg-gray-50 dark:bg-gray-800 p-2 rounded">
                         <div class="text-gray-500">上传流量</div>
                         <div class="font-mono">{{ formatBytes(node.systemInfo?.uploadTraffic) }}</div>
                     </div>
                     <div class="bg-gray-50 dark:bg-gray-800 p-2 rounded">
                         <div class="text-gray-500">下载流量</div>
                         <div class="font-mono">{{ formatBytes(node.systemInfo?.downloadTraffic) }}</div>
                     </div>
                 </div>

                 <!-- Footer Actions -->
                 <div class="flex gap-2 justify-end pt-2 border-t border-gray-100 dark:border-gray-800">
                     <NButton size="tiny" secondary type="success" @click="handleInstall(node)">安装</NButton>
                     <NButton size="tiny" secondary type="primary" @click="handleEditFunc(node)">编辑</NButton>
                     <NButton size="tiny" secondary type="error" @click="handleDelete(node)">删除</NButton>
                 </div>
             </div>
         </NCard>
     </div>

     <NModal v-model:show="showModal" preset="card" :title="isEdit ? '编辑节点' : '新增节点'" style="width: 600px">
         <NForm label-placement="left" label-width="100">
             <NFormItem label="节点名称"><NInput v-model:value="formModel.name" /></NFormItem>
             <NFormItem label="入口IPs">
                 <NInput v-model:value="formModel.ipString" type="textarea" placeholder="一行一个IP" />
             </NFormItem>
             <NFormItem label="服务器IP"><NInput v-model:value="formModel.serverIp" /></NFormItem>
             <NFormItem label="端口范围">
                 <div class="flex items-center gap-2">
                     <NInputNumber v-model:value="formModel.portSta" :min="1" />
                     <span>-</span>
                     <NInputNumber v-model:value="formModel.portEnd" :max="65535" />
                 </div>
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
