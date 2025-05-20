import {
  BadRequestException,
  ForbiddenException,
  Injectable,
  Logger,
} from '@nestjs/common';
import { TokenService } from './token.service';
import { AuthDto } from './auth.dto';
import { ConfigService } from '@nestjs/config';

@Injectable()
export class AuthService {
  private readonly logger = new Logger(AuthService.name, { timestamp: true });

  constructor(
    private readonly tokenService: TokenService,
    private readonly configService: ConfigService,
  ) {}

  async logIn(
    { account, password }: AuthDto,
    headers: Record<string, any>,
    ip: string,
  ): Promise<any> {
    const user = await this.configService.get('account');
    if (!user.account || !user.password) {
      throw new ForbiddenException('没有初始账户');
    }
    if (account !== user.account || password !== user.password) {
      throw new BadRequestException('账号密码错误');
    }
    const token = await this.tokenService.generateToken(headers, ip);
    this.logger.log(
      `Logged In: ${account} @ ${ip} By ${headers['user-agent']}`,
    );
    return { token };
  }
}
