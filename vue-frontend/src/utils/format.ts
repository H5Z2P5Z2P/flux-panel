import { useDateFormat } from '@vueuse/core';

export const formatFlow = (value: number, unit: string = 'bytes'): string => {
    if (value === 99999) return '无限制';

    if (unit === 'gb') {
        return value + ' GB';
    } else {
        if (value === 0) return '0 B';
        if (value < 1024) return value + ' B';
        if (value < 1024 * 1024) return (value / 1024).toFixed(2) + ' KB';
        if (value < 1024 * 1024 * 1024) return (value / (1024 * 1024)).toFixed(2) + ' MB';
        return (value / (1024 * 1024 * 1024)).toFixed(2) + ' GB';
    }
};

export const formatNumber = (value: number): string => {
    if (value === 99999) return '无限制';
    return value.toString();
};

export const formatResetTime = (resetDay?: number): string => {
    if (resetDay === undefined || resetDay === null) return '';
    if (resetDay === 0) return '不重置';

    const now = new Date();
    const currentDay = now.getDate();

    let daysUntilReset;
    if (resetDay > currentDay) {
        daysUntilReset = resetDay - currentDay;
    } else if (resetDay < currentDay) {
        const nextMonth = new Date(now.getFullYear(), now.getMonth() + 1, resetDay);
        const diffTime = nextMonth.getTime() - now.getTime();
        daysUntilReset = Math.ceil(diffTime / (1000 * 60 * 60 * 24));
    } else {
        daysUntilReset = 0;
    }

    if (daysUntilReset === 0) return '今日重置';
    if (daysUntilReset === 1) return '明日重置';
    return `${daysUntilReset}天后重置`;
};

export const getExpStatus = (expTime?: number) => {
    if (!expTime) return {
        color: 'text-green-600 dark:text-green-400',
        borderColor: 'border-green-200 dark:border-green-500/20',
        bg: 'bg-green-50 dark:bg-green-500/10',
        text: '永久'
    };

    const now = Date.now();

    if (expTime < now) {
        return {
            color: 'text-red-600 dark:text-red-400',
            borderColor: 'border-red-200 dark:border-red-500/20',
            bg: 'bg-red-50 dark:bg-red-500/10',
            text: '已过期'
        };
    }

    const diffTime = expTime - now;
    const diffDays = Math.ceil(diffTime / (1000 * 60 * 60 * 24));

    if (diffDays <= 7) {
        return {
            color: 'text-red-600 dark:text-red-400',
            borderColor: 'border-red-200 dark:border-red-500/20',
            bg: 'bg-red-50 dark:bg-red-200',
            text: `${diffDays}天后过期`
        };
    } else if (diffDays <= 30) {
        return {
            color: 'text-orange-600 dark:text-orange-400',
            borderColor: 'border-orange-200 dark:border-orange-500/20',
            bg: 'bg-orange-50 dark:bg-orange-500/10',
            text: `${diffDays}天后过期`
        };
    } else {
        return {
            color: 'text-green-600 dark:text-green-400',
            borderColor: 'border-green-200 dark:border-green-500/20',
            bg: 'bg-green-50 dark:bg-green-500/10',
            text: `${diffDays}天后过期`
        };
    }
};

export const formatAddress = (ipString: string, port: number): string => {
    if (!ipString || !port) return '';

    const ips = ipString.split(',').map(ip => ip.trim()).filter(ip => ip);

    if (ips.length === 0) return '';

    if (ips.length === 1) {
        const ip = ips[0];
        if (ip && ip.includes(':') && !ip.startsWith('[')) {
            return `[${ip}]:${port}`;
        } else {
            return `${ip}:${port}`;
        }
    }

    const firstIp = ips[0];
    let formattedFirstIp;

    if (firstIp && firstIp.includes(':') && !firstIp.startsWith('[')) {
        formattedFirstIp = `[${firstIp}]`;
    } else {
        formattedFirstIp = firstIp;
    }

    return `${formattedFirstIp}:${port} (+${ips.length - 1})`;
};

export const formatRemoteAddress = (remoteAddr: string): string => {
    if (!remoteAddr) return '';

    const addresses = remoteAddr.split(',').map(addr => addr.trim()).filter(addr => addr);

    if (addresses.length === 0) return '';

    if (addresses.length === 1) {
        return addresses[0] || '';
    }

    return `${addresses[0] || ''} (+${addresses.length - 1})`;
};

export const hasMultiple = (str: string): boolean => {
    if (!str) return false;
    return str.split(',').filter(s => s.trim()).length > 1;
};

export const getUsageColor = (percentage: number) => {
    if (percentage >= 90) return 'bg-red-500 dark:bg-red-600';
    if (percentage >= 70) return 'bg-orange-500 dark:bg-orange-600';
    return 'bg-blue-500 dark:bg-blue-600';
};
