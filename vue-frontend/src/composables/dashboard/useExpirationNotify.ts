import { useNotification } from 'naive-ui';
import type { User, UserTunnel } from '@/types';

export function useExpirationNotify() {
    const notification = useNotification();

    const checkAndNotify = (userInfo: User, tunnels: UserTunnel[]) => {
        const notificationKey = `expiration-${userInfo.expTime}-${tunnels.map(t => t.expTime).join(',')}`;
        const lastNotified = localStorage.getItem('lastNotified');

        if (lastNotified === notificationKey) {
            return;
        }

        let hasNotification = false;

        // Check Account Expiration
        if (userInfo.expTime) {
            const diffDays = getDiffDays(userInfo.expTime);
            if (diffDays <= 7) {
                hasNotification = true;
                notify(
                    '账户有效期提醒',
                    diffDays <= 0 ? '账户已过期，请立即续费' : `账户将于 ${diffDays} 天后过期，请及时续费`,
                    diffDays <= 0 ? 'error' : 'warning'
                );
            }
        }

        // Check Tunnels
        tunnels.forEach(tunnel => {
            if (tunnel.expTime) {
                const diffDays = getDiffDays(tunnel.expTime);
                if (diffDays <= 7) {
                    hasNotification = true;
                    notify(
                        `隧道 ${tunnel.tunnelName} 提醒`,
                        diffDays <= 0 ? `隧道已过期` : `将会于 ${diffDays} 天后过期`,
                        diffDays <= 0 ? 'error' : 'warning'
                    );
                }
            }
        });

        if (hasNotification) {
            localStorage.setItem('lastNotified', notificationKey);
        }
    };

    const getDiffDays = (expTime: number) => {
        const now = new Date();
        const expDate = new Date(expTime);
        if (isNaN(expDate.getTime())) return 999;

        // logic from original code: expDate > now check inside here or caller?
        // Original code: if expDate > now then calc diff. If diff <= 0 (means exactly now or just passed?)
        // Actually the logic `diffDays <= 0` only happens if `expDate <= now` usually. 
        // Let's stick to simple timestamp diff.
        const diffTime = expTime - now.getTime();
        return Math.ceil(diffTime / (1000 * 60 * 60 * 24));
    };

    const notify = (title: string, content: string, type: 'warning' | 'error') => {
        notification[type]({
            title,
            content,
            duration: type === 'error' ? 8000 : 5000,
            keepAliveOnHover: true
        });
    };

    return {
        checkAndNotify
    };
}
