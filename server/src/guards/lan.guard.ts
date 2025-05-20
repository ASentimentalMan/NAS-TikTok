import { isLAN } from '@/utils/tool.utils';
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
export class LANGuard implements CanActivate {
  private readonly logger = new Logger(LANGuard.name, { timestamp: true });

  constructor(private readonly configService: ConfigService) {}

  async canActivate(context: ExecutionContext): Promise<boolean> {
    const lanOnly = this.configService.get('server')?.lan;
    if (lanOnly) {
      const request = context.switchToHttp().getRequest<
        FastifyRequest & {
          query: { auth?: string };
          jwt: Record<string, any>;
        }
      >();
      const isFromLan = isLAN(request.ip);
      if (!isFromLan) {
        this.logger.log(`禁止访问 @ ${request.ip} `);
        throw new ForbiddenException(`禁止访问`);
      }
    }
    return true;
  }
}

export const LANProvider = {
  provide: APP_GUARD,
  useClass: LANGuard,
};
