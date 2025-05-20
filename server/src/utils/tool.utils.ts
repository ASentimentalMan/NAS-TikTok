import { isIPv4, isIPv6 } from 'net';

export const generateUUID = (): string => {
  return 'xxxxxxxx-xxxx-xxxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    let r = (Math.random() * 16) | 0,
      v = c == 'x' ? r : (r & 0x3) | 0x8;
    return v.toString(16);
  });
};

export const isLAN = (ip: string): boolean => {
  ip = ip.toLowerCase();

  if (ip === 'localhost' || ip === '::1') return true;

  if (isIPv4(ip)) {
    const parts = ip.split('.').map(Number);
    const [a, b] = parts;
    return (
      a === 10 ||
      (a === 172 && b >= 16 && b <= 31) ||
      (a === 192 && b === 168) ||
      a === 127
    );
  }

  if (isIPv6(ip)) {
    if (ip.startsWith('fe80')) return true;
    const prefix = parseInt(ip.slice(0, 2), 16);
    if ((prefix & 0xfe) === 0xfc) return true;
  }

  return false;
};
