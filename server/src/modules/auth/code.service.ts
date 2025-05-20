import { BadRequestException, Inject, Injectable } from '@nestjs/common';
import { Cache, CACHE_MANAGER } from '@nestjs/cache-manager';
import * as svgCaptcha from 'svg-captcha';
import { generateUUID } from '@/utils/tool.utils';

@Injectable()
export class CodeService {
  constructor(@Inject(CACHE_MANAGER) private readonly cacheManager: Cache) {}

  /* 获得Redis没有的UUID */
  private async getUniqueUUID() {
    const id = generateUUID();
    const existence = await this.cacheManager.get(id);
    if (existence) {
      return await this.getUniqueUUID();
    }
    return { id };
  }

  /* 生成图片验证码 */
  async captcha(): Promise<any> {
    const svg = svgCaptcha.createMathExpr({
      size: 4,
      ignoreChars: '0o1Il',
      noise: 4,
      color: true,
      background: 'transparent',
      width: 100,
      height: 32,
      fontSize: 48,
    });
    const { id } = await this.getUniqueUUID();
    const value = svg.text;
    await this.cacheManager.set(id, value, 5 * 60 * 1000);
    return {
      id,
      data: `data:image/svg+xml;base64,${Buffer.from(svg.data).toString(
        'base64',
      )}`,
    };
  }

  /* 校验验证码 */
  async checkVerifyCode(id: string, code: string): Promise<void> {
    const result: string | undefined =
      (await this.cacheManager.get(id)) || undefined;
    // 校验后移除验证码
    if (result) await this.cacheManager.del(id);
    if (!result || code.toLowerCase() !== result.toLowerCase()) {
      throw new BadRequestException('验证码错误');
    }
  }
}
