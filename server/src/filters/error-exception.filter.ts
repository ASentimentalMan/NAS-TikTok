import {
  Catch,
  ExceptionFilter,
  ArgumentsHost,
  HttpStatus,
  Logger,
} from '@nestjs/common';
import { FastifyRequest, FastifyReply } from 'fastify';

@Catch(Error)
export class ErrorExceptionsFilter implements ExceptionFilter {
  private readonly logger = new Logger(ErrorExceptionsFilter.name, {
    timestamp: true,
  });

  catch(exception: any, host: ArgumentsHost) {
    const ctx = host.switchToHttp();
    const request = ctx.getRequest<FastifyRequest>();
    const response = ctx.getResponse<FastifyReply>();

    // 输出日志
    this.logger.error(
      `HTTP [${request.method}] [${request.url}] ${exception.message}`,
    );

    response.status(HttpStatus.INTERNAL_SERVER_ERROR).send({
      code: HttpStatus.INTERNAL_SERVER_ERROR,
      data: null,
      message: exception.message,
    });
  }
}
