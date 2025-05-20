import { Module } from '@nestjs/common';
import { ConfigModule } from '@nestjs/config';
import { ServerConfig } from './configs/server.config';
import { StreamModule } from './modules/stream/stream.modules';
import { ServeStaticModule } from '@nestjs/serve-static';
import { AccountConfig } from './configs/account.config';
import { CacheModule } from '@nestjs/cache-manager';
import { join } from 'path';
import { AuthModule } from './modules/auth/auth.module';
import { JWTConfig } from './configs/jwt.config';
import { JWTProvider } from './guards/jwt.guard';
import { LANProvider } from './guards/lan.guard';

@Module({
  imports: [
    ConfigModule.forRoot({
      isGlobal: true,
      envFilePath: ['.env.local', `.env.${process.env.NODE_ENV}`, '.env'],
      load: [ServerConfig, AccountConfig, JWTConfig],
    }),
    CacheModule.register({ isGlobal: true }),
    ServeStaticModule.forRoot({
      rootPath:
        process.env.NODE_ENV === 'development'
          ? join(__dirname, '../..', 'client', 'dist')
          : '/workspace/client/dist',
    }),
    AuthModule,
    StreamModule,
  ],
  controllers: [],
  providers: [
    JWTProvider,
    LANProvider
  ],
})
export class AppModule {}
