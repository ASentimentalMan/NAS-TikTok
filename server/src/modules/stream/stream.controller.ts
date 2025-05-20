import { Controller, Get, Headers, Param, Query, Res } from '@nestjs/common';
import { StreamService } from './stream.service';

@Controller()
export class StreamController {
  constructor(private readonly streamService: StreamService) {}

  @Get('list')
  async list(@Query() query: { type: 'image' | 'video' }) {
    return this.streamService.list(query);
  }

  @Get('stream/*')
  async streamPublicFile(
    @Param('*') file: string,
    @Res() res: any,
    @Headers('range') range?: string, // 获取 Range 请求头
  ) {
    return this.streamService.streamFile(file, res, range);
  }
}
