import { isIPv4, isIPv6 } from 'net';

export const generateUUID = (): string => {
  return 'xxxxxxxx-xxxx-xxxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    let r = (Math.random() * 16) | 0,
      v = c == 'x' ? r : (r & 0x3) | 0x8;
    return v.toString(16);
  });
};

/**
 * 将 IPv4 字符串转为 32 位无符号整数
 */
const ip4ToNum = (ip: string): number => {
  const parts = ip.split('.');
  if (parts.length !== 4) throw new Error('Invalid IPv4');
  return parts.reduce((acc, octet) => (acc << 8) | parseInt(octet, 10), 0) >>> 0;
};

/**
 * 判断 IP 是否在允许的列表或局域网范围内
 * @param ip 待检测 IP
 * @param AllowedIPs 逗号分隔的字符串，如 "192.168.0.0/16, 1.1.1.1"
 */
export const isAllowedIPs = (ip: string, AllowedIPs: string = '', allowIPv6 = false): boolean => {
  const targetIp = ip.toLowerCase().trim();

  // 1. 基础本地环回判断
  if (targetIp === 'localhost' || targetIp === '127.0.0.1' || targetIp === '::1') return true;

  // 2. 解析传入的 ALLOWED_IPS 字符串为数组
  const extraConfig = AllowedIPs
    .split(',')
    .map(s => s.trim())
    .filter(Boolean);

  // 3. IPv4 逻辑
  if (isIPv4(targetIp)) {
    try {
      const targetNum = ip4ToNum(targetIp);

      // 默认本地环回网段 + 传入的自定义配置
      const allSubnets = [
        '127.0.0.0/8',
        ...extraConfig
      ];

      return allSubnets.some(cidr => {
        // 跳过非 IPv4 格式的配置项 (防止混入 IPv6)
        if (!cidr.includes('.')) return false;

        const [range, bits = '32'] = cidr.split('/');
        const maskBits = parseInt(bits, 10);

        // 生成掩码 (利用无符号右移保证 32 位一致性)
        const mask = maskBits === 0 ? 0 : (-1 << (32 - maskBits)) >>> 0;

        return (targetNum & mask) === (ip4ToNum(range.trim()) & mask);
      });
    } catch {
      return false;
    }
  }

  // 4. IPv6 逻辑
  else if (allowIPv6 && isIPv6(targetIp)) return true;

  return false;
};
