// 用户管理相关类型
export interface User {
    id: number;
    name?: string;
    user: string;
    pwd?: string;
    status: number; // 1-正常, 0-禁用
    flow: number; // 流量限制(GB)
    num: number; // 转发数量
    expTime?: number; // 过期时间戳
    flowResetTime?: number; // 流量重置日期(1-31号)
    createdTime?: number; // 创建时间戳
    inFlow?: number; // 下载流量(字节)
    outFlow?: number; // 上传流量(字节)
}

export interface UserForm {
    id?: number;
    name?: string;
    user: string;
    pwd?: string;
    status: number;
    flow: number;
    num: number;
    expTime: number | null;
    flowResetTime: number;
}

export interface UserTunnel {
    id: number;
    userId: number;
    tunnelId: number;
    tunnelName: string;
    status: number; // 1-正常, 0-禁用
    speedId?: number | null; // 限速规则ID
    speedLimitName?: string; // 限速规则名称
    inFlow?: number; // 下载流量(字节)
    outFlow?: number; // 上传流量(字节)
    tunnelFlow?: number; // 隧道流量计算类型(1-单向, 2-双向)
    expTime?: number;
    flow?: number;
    num?: number;
    flowResetTime?: number;
}

export interface UserTunnelForm {
    tunnelId: number | null;
    speedId: number | null;
}

export interface Tunnel {
    id: number;
    name: string;
    entryNodeId?: number; // legacy
    exitNodeId?: number; // legacy
    inNodeId?: number;
    outNodeId?: number;
    entryNodeName?: string;
    exitNodeName?: string;
    status?: number;
    flow?: number; // 流量计算类型
    type?: number;
    trafficRatio?: number;
    tcpListenAddr?: string;
    udpListenAddr?: string;
    interfaceName?: string;
    protocol?: string;
}

export interface SpeedLimit {
    id: number;
    name: string;
    tunnelId: number;
    uploadSpeed: number;
    downloadSpeed: number;
}

export interface Pagination {
    current: number;
    size: number;
    total: number;
}
