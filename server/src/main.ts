import { NestFactory } from '@nestjs/core';
import { AppModule } from './app.module';
import {
  FastifyAdapter,
  NestFastifyApplication,
} from '@nestjs/platform-fastify';
import { ConfigService } from '@nestjs/config';
import { ErrorExceptionsFilter, HttpExceptionFilter } from './filters';
import { readFileSync } from 'fs';
import cluster from 'cluster';
import { ValidationPipe } from '@nestjs/common';

const bootstrap = async () => {
  const fastifyOptions: Record<string, any> = {};
  if (process.env.SSL === 'true') {
    fastifyOptions.https = {
      key: readFileSync('/workspace/cert/privkey.pem'),
      cert: readFileSync('/workspace/cert/fullchain.pem'),
    };
    fastifyOptions.http2 = true;
  }
  const app = await NestFactory.create<NestFastifyApplication>(
    AppModule,
    new FastifyAdapter(fastifyOptions),
  );
  const appConf = app.get(ConfigService)?.get('server');
  if (!appConf) return;
  app.enableCors({
    origin: '*',
    credentials: true,
    // 明确允许方法
    methods: ['GET', 'OPTIONS'],
  });
  app.setGlobalPrefix('api');
  app.useGlobalFilters(new ErrorExceptionsFilter(), new HttpExceptionFilter());
  app.useGlobalPipes(
    new ValidationPipe({
      stopAtFirstError: true,
      whitelist: true,
      // 禁止 无装饰器验证的数据通过
      forbidUnknownValues: true,
      transform: true,
    }),
  );
  process.on('uncaughtException', (e) => {
    console.error(e);
  });
  process.on('unhandledRejection', (reason) => {
    console.warn(reason);
  });
  await app.listen(appConf.port, appConf.host, async () => {
    const isMainCluster =
      process?.env?.NODE_APP_INSTANCE &&
      Number.parseInt(process?.env?.NODE_APP_INSTANCE) === 0;
    const isMainProcess = cluster?.isPrimary || isMainCluster;
    if (!isMainProcess) return;
    console.log(
      `${appConf.appName} is running on ${appConf.appURL}:${appConf.port}${appConf.urlPrefix}`,
    );
  });
};
bootstrap();
