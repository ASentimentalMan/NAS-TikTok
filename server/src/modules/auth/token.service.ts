import { generateUUID } from '@/utils/tool.utils';
import { Injectable } from '@nestjs/common';
import { JwtService } from '@nestjs/jwt';

/* 令牌服务 */
@Injectable()
export class TokenService {
  constructor(private jwtService: JwtService) {}

  async generateToken(
    headers: Record<string, any>,
    ip: string,
  ): Promise<string> {
    const session = generateUUID();
    const {
      host,
      connection,
      'user-agent': userAgent,
      dnt,
      'sec-fetch-site': secFetchSite,
      'accept-language': acceptLanguage,
    } = headers;
    const jwt = Object.assign(
      {
        host,
        connection,
        'user-agent': userAgent,
        dnt,
        'sec-fetch-site': secFetchSite,
        'accept-language': acceptLanguage,
      },
      { session, ip },
    );
    const token = await this.jwtService.signAsync(jwt);
    return token;
  }
}
