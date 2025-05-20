import { createParamDecorator } from '@nestjs/common';

export const JWT = createParamDecorator((data, req) => {
  const request = req.switchToHttp().getRequest();
  return data ? request.jwt?.[data] : request.jwt;
});
