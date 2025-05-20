import {
  Body,
  Controller,
  Ip,
  Post,
  Headers,
  Get,
  UseGuards,
} from '@nestjs/common';
import { AuthService } from './auth.service';
import { Public } from '@/decorators/public.decorator';
import { AuthDto } from './auth.dto';
import { CodeService } from './code.service';
import { CodeGuard } from '@/guards/code.guard';

@Controller()
export class AuthController {
  constructor(
    private readonly authService: AuthService,
    private readonly codeService: CodeService,
  ) {}

  @Public()
  @Get('captcha')
  captcha(): any {
    return this.codeService.captcha();
  }

  @Public()
  @Post('logIn')
  @UseGuards(CodeGuard)
  async logIn(
    @Body() dto: AuthDto,
    @Headers() headers: Record<string, any>,
    @Ip() ip: string,
  ): Promise<any> {
    return this.authService.logIn(dto, headers, ip);
  }
}
