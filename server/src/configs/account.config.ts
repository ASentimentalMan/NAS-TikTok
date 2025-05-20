import { registerAs } from '@nestjs/config';

export const AccountConfig = registerAs(
  'account',
  (): Record<string, any> => ({
    account: String(process?.env?.ACCOUNT),
    password: String(process?.env?.PASSWORD),
  }),
);
