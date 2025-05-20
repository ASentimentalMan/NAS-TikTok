import {
  BadRequestException,
  CanActivate,
  ExecutionContext,
  Injectable,
} from '@nestjs/common';
import { FastifyRequest } from 'fastify';
import { CodeService } from '@/modules/auth/code.service';

@Injectable()
export class CodeGuard implements CanActivate {
  constructor(
    private readonly codeService: CodeService,
  ) {}

  async canActivate(context: ExecutionContext): Promise<boolean> {
    const request = context.switchToHttp().getRequest<FastifyRequest>();
    const body = request.body as { codeId?: string; code?: string };
    if (!body.codeId || !body.code) {
      throw new BadRequestException("请输入验证码");
    }
    await this.codeService.checkVerifyCode(body.codeId, body.code);
    return true;
  }
}
