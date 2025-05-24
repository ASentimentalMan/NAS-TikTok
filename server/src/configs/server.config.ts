import { registerAs } from '@nestjs/config';

export const ServerConfig = registerAs(
  'server',
  (): Record<string, any> => ({
    appName: String(process?.env?.APP_NAME),
    appURL: String(process?.env?.VITE_API_URL),
    urlPrefix: String(process?.env?.APP_URL_PREFIX),
    host: String(process?.env?.IPv6 == 'true' ? '::' : '0.0.0.0'),
    port: Number(process?.env?.VITE_API_PORT),
    staticURL: `${String(process?.env?.VITE_API_URL)}:${Number(process?.env?.VITE_API_PORT)}${String(process?.env?.APP_URL_PREFIX)}/stream`,
    lan: Boolean(process?.env?.LAN_ONLY == 'true'),
  }),
);
