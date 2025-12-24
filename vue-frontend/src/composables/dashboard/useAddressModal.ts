import { ref } from 'vue';
import { useClipboard } from '@vueuse/core';
import { useMessage } from 'naive-ui';

export interface AddressItem {
    id: number;
    ip: string;
    address: string;
}

export function useAddressModal() {
    const isOpen = ref(false);
    const title = ref('');
    const addressList = ref<AddressItem[]>([]);

    const { copy, isSupported } = useClipboard();
    const message = useMessage();

    const openModal = async (rawString: string, basePort: number | null, modalTitle: string) => {
        if (!rawString) return;

        const items = rawString.split(',').map(s => s.trim()).filter(Boolean);

        // Single item -> Direct Copy
        if (items.length <= 1) {
            // Logic for single copy formatting
            let textToCopy = rawString;
            if (basePort) { // It's an entrance address
                textToCopy = formatSingleEntrance(items[0] || '', basePort);
            }
            await handleCopy(textToCopy);
            return;
        }

        // Multiple -> Open Modal
        title.value = `${modalTitle} (${items.length}个)`;
        addressList.value = items.map((ip, idx) => ({
            id: idx,
            ip,
            address: basePort ? formatSingleEntrance(ip, basePort) : ip
        }));
        isOpen.value = true;
    };

    const formatSingleEntrance = (ip: string, port: number) => {
        if (ip.includes(':') && !ip.startsWith('[')) {
            return `[${ip}]:${port}`;
        }
        return `${ip}:${port}`;
    };

    const handleCopy = async (text: string) => {
        if (!isSupported) {
            message.error('浏览器不支持自动复制');
            return;
        }
        await copy(text);
        message.success('已复制');
    };

    const copyItem = async (item: AddressItem) => {
        await handleCopy(item.address);
    };

    const copyAll = async () => {
        const allText = addressList.value.map(i => i.address).join('\n');
        await handleCopy(allText);
    };

    return {
        isOpen,
        title,
        addressList,
        openModal,
        copyItem,
        copyAll
    };
}
