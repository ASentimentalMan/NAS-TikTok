import { isAllowedIPs } from '@/utils/tool.utils';
import {
  CanActivate,
  ExecutionContext,
  ForbiddenException,
  Injectable,
  Logger,
} from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import { APP_GUARD } from '@nestjs/core';
import { FastifyRequest } from 'fastify';

@Injectable()
export class IPGuard implements CanActivate {
  private readonly logger = new Logger(IPGuard.name, { timestamp: true });

  constructor(private readonly configService: ConfigService) {}

  async canActivate(context: ExecutionContext): Promise<boolean> {
    const allowedIPs = this.configService.get('server')?.allowedIPs;
    const allowIPv6 = this.configService.get('server')?.allowIPv6;
    if (allowedIPs) {
      const request = context.switchToHttp().getRequest<
        FastifyRequest & {
          query: { auth?: string };
          jwt: Record<string, any>;
        }
      >();
      const isInAllowedIPs = isAllowedIPs(request.ip, allowedIPs, allowIPv6);
      if (!isInAllowedIPs) {
        this.logger.log(`禁止访问 @ ${request.ip} `);
        throw new ForbiddenException(`禁止访问`);
      }
    }
    return true;
  }
}

export const IPProvider = {
  provide: APP_GUARD,
  useClass: IPGuard,
};
