import type { ArgumentsHost, ExceptionFilter } from '@nestjs/common';
import { Catch, HttpException, Logger } from '@nestjs/common';
import { FastifyRequest, FastifyReply } from 'fastify';

@Catch(HttpException)
export class HttpExceptionFilter implements ExceptionFilter {
  private logger = new Logger(HttpExceptionFilter.name, { timestamp: true });
  catch(exception: HttpException, host: ArgumentsHost) {
    const ctx = host.switchToHttp();
    const request = ctx.getRequest<FastifyRequest>();
    const response = ctx.getResponse<FastifyReply>();
    const status = exception.getStatus();

    let message: string =
      (exception.getResponse() as any).message || exception.message;

    // 输出日志
    this.logger.error(`HTTP [${request.method}] [${request.url}] ${message}`);

    response.status(status).send({
      code: status,
      data: null,
      message,
    });
  }
}
