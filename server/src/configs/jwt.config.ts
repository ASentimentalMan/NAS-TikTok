import { registerAs } from '@nestjs/config';

export const JWTConfig = registerAs(
  'jwt',
  (): Record<string, any> => ({
    accessSecret: String(process?.env?.JWT_SECRET),
    accessExpiresIn: Number(process?.env?.JWT_EXPIRE),
  })
);
