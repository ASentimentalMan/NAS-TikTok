import { Module } from '@nestjs/common';
import { AuthController } from './auth.controller';
import { AuthService } from './auth.service';
import { TokenService } from './token.service';
import { JwtModule } from '@nestjs/jwt';
import { ConfigModule, ConfigService } from '@nestjs/config';
import { CodeService } from './code.service';

@Module({
  controllers: [AuthController],
  providers: [AuthService, TokenService, CodeService],
  imports: [
    JwtModule.registerAsync({
      global: true,
      imports: [ConfigModule],
      useFactory: (configService: ConfigService) => {
        const { accessSecret, accessExpiresIn } =
          configService.get<Record<string, any>>('jwt') || {};
        return {
          secret: accessSecret,
          signOptions: {
            expiresIn: `${accessExpiresIn}s`,
          },
        };
      },
      inject: [ConfigService],
    }),
  ],
  exports: [],
})
export class AuthModule {}
