import { IS_PUBLIC_KEY } from '@/decorators/public.decorator';
import { Cache, CACHE_MANAGER } from '@nestjs/cache-manager';
import {
  CanActivate,
  ExecutionContext,
  Inject,
  Injectable,
  UnauthorizedException,
} from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import { APP_GUARD, Reflector } from '@nestjs/core';
import { JwtService } from '@nestjs/jwt';
import { FastifyRequest } from 'fastify';

@Injectable()
export class JWTGuard implements CanActivate {
  constructor(
    private reflector: Reflector,
    private readonly jwtService: JwtService,
    private readonly configService: ConfigService,
    @Inject(CACHE_MANAGER) private readonly cacheManager: Cache,
  ) {}

  async canActivate(context: ExecutionContext): Promise<boolean> {
    const isPublic = this.reflector.getAllAndOverride<boolean>(IS_PUBLIC_KEY, [
      context.getHandler(),
      context.getClass(),
    ]);
    if (isPublic) {
      return true;
    }

    const request = context.switchToHttp().getRequest<
      FastifyRequest & {
        query: { auth?: string };
        jwt: Record<string, any>;
      }
    >();

    let token: string | undefined;
    const urlPrefix = this.configService.get('server')?.urlPrefix;
    if (request.url.startsWith(`${urlPrefix}/stream/`)) {
      token = request.query.auth;
    } else {
      token = this.extractTokenFromHeader(request);
    }

    if (!token) {
      throw new UnauthorizedException('用户未登录');
    }
    try {
      const jwt: JWT = await this.jwtService.verifyAsync(token);
      if (!this.verifyHeaders(request.headers, jwt, request.ip)) {
        throw new UnauthorizedException('登录已过期');
      }
      request.jwt = jwt;
    } catch {
      throw new UnauthorizedException('登录已过期');
    }
    return true;
  }

  private verifyHeaders(
    headers: Record<string, any>,
    jwt: JWT,
    ip: string,
  ): boolean {
    const {
      host,
      connection,
      'user-agent': userAgent,
      dnt,
      'sec-fetch-site': secFetchSite,
      'accept-language': acceptLanguage,
    } = headers;

    // 定义需要对比的字段列表
    const fieldsToCheck = [
      { header: host, jwt: jwt.host, name: 'host' },
      { header: connection, jwt: jwt.connection, name: 'connection' },
      { header: userAgent, jwt: jwt['user-agent'], name: 'user-agent' },
      { header: dnt, jwt: jwt.dnt, name: 'dnt' },
      {
        header: secFetchSite,
        jwt: jwt['sec-fetch-site'],
        name: 'sec-fetch-site',
      },
      {
        header: acceptLanguage,
        jwt: jwt['accept-language'],
        name: 'accept-language',
      },
      { header: ip, jwt: jwt.ip, name: 'ip' },
    ];

    // 检查每个字段
    for (const { header, jwt: jwtValue, name } of fieldsToCheck) {
      if (jwt.hasOwnProperty(name) && String(header) !== String(jwtValue)) {
        console.log('header-not-match', name, header, jwtValue);
        return false;
      }
    }

    return true;
  }

  private extractTokenFromHeader(request: FastifyRequest): string | undefined {
    const [type, token] = request.headers.authorization?.split(' ') ?? [];
    return type === 'Bearer' ? token : undefined;
  }
}

export const JWTProvider = {
  provide: APP_GUARD,
  useClass: JWTGuard,
};
