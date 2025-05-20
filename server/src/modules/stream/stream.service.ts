import { Injectable, NotFoundException } from '@nestjs/common';
import { createReadStream, Dir, Dirent, existsSync, statSync } from 'fs';
import { basename, extname, join } from 'path';
import { lookup } from 'mime-types';
import { ConfigService } from '@nestjs/config';
import { opendir } from 'fs/promises';
import { randomInt } from 'crypto';

@Injectable()
export class StreamService {
  private readonly workDir: string;
  constructor(private readonly configService: ConfigService) {
    this.workDir =
      process.env.NODE_ENV === 'development'
        ? join(__dirname, '../../..', 'statics')
        : '/workspace/statics';
  }

  private readonly VIDEO_EXTENSIONS = new Set([
    '.mp4',
    '.webm',
    '.mov',
    '.ogv',
    '.ogg',
  ]);
  private readonly IMAGE_EXTENSIONS = new Set([
    '.jpg',
    '.jpeg',
    '.heic',
    '.heif',
    '.avif',
    '.webp',
    '.png',
    '.gif',
    '.apng',
    '.bmp',
  ]);
  // 辅助方法：根据文件扩展名获取 Content-Type
  private readonly getContentType = (ext: string): string => {
    const typeMap: Record<string, string> = {
      '.jpg': 'image/jpeg',
      '.jpeg': 'image/jpeg',
      '.png': 'image/png',
      '.gif': 'image/gif',
      '.webp': 'image/webp',
      '.bmp': 'image/bmp',
      '.svg': 'image/svg+xml',
      '.mp4': 'video/mp4',
      '.webm': 'video/webm',
      '.mov': 'video/quicktime',
    };
    return typeMap[ext] || lookup(ext) || 'application/octet-stream';
  };
  
  async list({ type }: { type: 'image' | 'video' }) {
    const extSet =
      type === 'image' ? this.IMAGE_EXTENSIONS : this.VIDEO_EXTENSIONS;
    const pageSize = 10;
    const rootPath = this.workDir;
    const staticURL = this.configService.get('server.staticURL');
    const selected: string[] = [];
    let seen = 0;

    const traverse = async (dir: string) => {
      let dirHandle: Dir | undefined;

      try {
        dirHandle = await opendir(dir);
        let dirent: Dirent | null;

        while ((dirent = await dirHandle.read()) !== null) {
          const fullPath = join(dir, dirent.name);

          if (dirent.isDirectory()) {
            if (dirent.name === '@eaDir') continue;
            await traverse(fullPath); // safe because we don’t interleave iteration
          } else if (dirent.isFile()) {
            const ext = extname(dirent.name).toLowerCase();
            if (!extSet.has(ext)) continue;

            seen++;
            const filePath = fullPath.replace(rootPath, '');
            if (selected.length < pageSize) {
              selected.push(filePath);
            } else {
              const j = randomInt(seen);
              if (j < pageSize) {
                selected[j] = filePath;
              }
            }
          }
        }
      } catch (err) {
        console.error(`Error reading directory ${dir}:`, err);
      } finally {
        await dirHandle?.close(); // safe close
      }
    };

    try {
      await traverse(rootPath);
    } catch (err) {
      console.error('Error during traversal:', err);
    }

    const list = selected.map(
      (e) => `${staticURL}/${encodeURIComponent(e.slice(1))}`,
    );
    return { data: list };
  }

  streamFile(filePath: string, res: any, range?: string) {
    try {
      // 解码 URL 编码的路径（如 %20 -> 空格）
      const decodedPath = decodeURIComponent(filePath);
      const path = join(this.workDir, decodedPath);

      // 检查文件是否存在
      if (!existsSync(path)) {
        throw new NotFoundException('File not found');
      }

      // 获取文件信息
      const stat = statSync(path);
      const fileSize = stat.size;
      const ext = extname(path).toLowerCase();
      const filename = basename(path);

      // 根据文件扩展名设置更精确的 Content-Type
      const contentType =
        this.getContentType(ext) || 'application/octet-stream';

      // 设置通用响应头
      res.raw.setHeader('Access-Control-Allow-Origin', '*');
      res.raw.setHeader('Access-Control-Allow-Methods', 'GET, HEAD');
      res.raw.setHeader(
        'Access-Control-Expose-Headers',
        'Content-Length,Content-Range',
      );
      res.raw.setHeader('Access-Control-Allow-Headers', 'Range');
      res.raw.setHeader('Accept-Ranges', 'bytes');
      const safeFilename = encodeURIComponent(filename);
      res.raw.setHeader(
        'Content-Disposition',
        `inline; filename="${safeFilename}"`,
      );

      // 处理 HEAD 请求
      if (res.request.method === 'HEAD') {
        res.raw.setHeader('Content-Type', contentType);
        res.raw.setHeader('Content-Length', fileSize);
        return res.raw.end();
      }

      // 非范围请求处理
      if (!range) {
        res.raw.setHeader('Content-Type', contentType);
        res.raw.setHeader('Content-Length', fileSize);
        return createReadStream(path).pipe(res.raw);
      }

      // 处理范围请求
      const parts = range.replace(/bytes=/, '').split('-');
      const start = parseInt(parts[0], 10);
      let end = parts[1] ? parseInt(parts[1], 10) : fileSize - 1;

      // 验证范围有效性
      if (start >= fileSize || end >= fileSize) {
        res.raw.setHeader('Content-Range', `bytes */${fileSize}`);
        return res.status(416).end();
      }

      // 调整结束位置
      end = Math.min(end, fileSize - 1);
      const contentLength = end - start + 1;

      // 设置部分内容响应头
      res.raw.writeHead(206, {
        'Content-Type': contentType,
        'Content-Length': contentLength,
        'Content-Range': `bytes ${start}-${end}/${fileSize}`,
        'Cache-Control': 'public, max-age=31536000', // 图片适合长期缓存
      });

      // 创建并返回文件流
      const stream = createReadStream(path, { start, end });
      stream.on('error', (err) => {
        console.error('Stream error:', err);
        if (!res.raw.headersSent) {
          res.status(500).send('Internal Server Error');
        }
      });
      stream.pipe(res.raw);
    } catch (err) {
      console.error('Error streaming file:', err);
      if (!res.raw.headersSent) {
        if (err instanceof NotFoundException) {
          res.status(404).send('File not found');
        } else {
          res.status(500).send('Internal Server Error');
        }
      }
    }
  }
}
