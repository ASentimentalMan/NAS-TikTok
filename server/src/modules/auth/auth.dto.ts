import { Type } from 'class-transformer';
import { IsNotEmpty, IsOptional } from 'class-validator';

export class VerifyCodeDto {
  @IsOptional()
  @Type(() => String)
  codeId?: string;

  @IsOptional()
  @Type(() => String)
  code?: string;
}

export class AuthDto extends VerifyCodeDto {
  @IsNotEmpty()
  @Type(() => String)
  account: string;

  @IsNotEmpty()
  @Type(() => String)
  password: string;
}
