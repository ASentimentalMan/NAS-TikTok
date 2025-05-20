import { Module } from "@nestjs/common";
import { StreamController } from "./stream.controller";
import { StreamService } from "./stream.service";

@Module({
  controllers: [StreamController],
  providers: [StreamService],
  imports: [],
  exports: [],
})
export class StreamModule {}
