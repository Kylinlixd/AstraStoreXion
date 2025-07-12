import { readFileSync } from 'fs';
import { parse } from 'yaml';

/**
 * 星辰离子X客户端配置
 */
export interface XionConfig {
  /**
   * API网关地址
   */
  apiGateway: string;

  /**
   * 请求超时时间（毫秒）
   */
  timeout: number;

  /**
   * 最大重试次数
   */
  maxRetries: number;

  /**
   * 重试间隔（毫秒）
   */
  retryInterval: number;

  /**
   * 分块大小（字节）
   */
  chunkSize: number;

  /**
   * 并行上传数
   */
  parallelUploads: number;

  /**
   * 并行下载数
   */
  parallelDownloads: number;

  /**
   * 是否启用缓存
   */
  enableCache: boolean;

  /**
   * 缓存有效期（毫秒）
   */
  cacheTTL: number;

  /**
   * 缓存最大大小（字节）
   */
  cacheMaxSize: number;
}

/**
 * 默认配置
 */
export const defaultConfig: XionConfig = {
  apiGateway: 'http://localhost:8080',
  timeout: 30000,
  maxRetries: 3,
  retryInterval: 1000,
  chunkSize: 4 * 1024 * 1024, // 4MB
  parallelUploads: 3,
  parallelDownloads: 3,
  enableCache: true,
  cacheTTL: 60000,
  cacheMaxSize: 100 * 1024 * 1024 // 100MB
};

/**
 * 从YAML文件加载配置
 * 
 * @param path 配置文件路径
 * @returns 客户端配置
 */
export function loadConfigFromYaml(path: string): XionConfig {
  try {
    const fileContent = readFileSync(path, 'utf8');
    const parsedConfig = parse(fileContent);
    return {
      ...defaultConfig,
      ...parsedConfig
    };
  } catch (err) {
    console.error(`无法加载配置文件: ${err}`);
    return defaultConfig;
  }
} 