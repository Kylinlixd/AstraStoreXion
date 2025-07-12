import axios, { AxiosInstance, AxiosRequestConfig } from 'axios';
import FormData from 'form-data';
import { Readable } from 'stream';
import { XionConfig, defaultConfig } from './config';
import { DeleteFileResponse, FileStatusResponse, UploadFileResponse } from './models';

/**
 * 星辰离子X客户端
 */
export class XionClient {
  private readonly config: XionConfig;
  private readonly axiosInstance: AxiosInstance;

  /**
   * 创建一个新的客户端实例
   * 
   * @param config 客户端配置
   */
  constructor(config: Partial<XionConfig> = {}) {
    this.config = { ...defaultConfig, ...config };
    
    // 创建HTTP客户端
    this.axiosInstance = axios.create({
      baseURL: this.config.apiGateway,
      timeout: this.config.timeout,
      headers: {
        'Content-Type': 'application/json',
        'Accept': 'application/json'
      }
    });

    // 添加重试逻辑的拦截器
    this.axiosInstance.interceptors.response.use(
      response => response,
      async error => {
        const config = error.config as AxiosRequestConfig & { _retryCount?: number };
        if (!config || !config.url) {
          return Promise.reject(error);
        }

        config._retryCount = config._retryCount || 0;
        
        if (config._retryCount < this.config.maxRetries) {
          config._retryCount += 1;
          
          // 等待重试间隔时间
          await new Promise(resolve => setTimeout(resolve, this.config.retryInterval));
          
          return this.axiosInstance(config);
        }

        return Promise.reject(error);
      }
    );
  }

  /**
   * 上传文件
   * 
   * @param fileStream 文件流
   * @param filename 文件名
   * @param metadata 元数据
   * @returns 上传响应
   */
  public async uploadFile(
    fileStream: Buffer | Readable,
    filename: string,
    metadata?: Record<string, string>
  ): Promise<UploadFileResponse> {
    const formData = new FormData();
    
    // 添加文件
    formData.append('file', fileStream, { filename });
    
    // 添加元数据
    if (metadata && Object.keys(metadata).length > 0) {
      formData.append('metadata', JSON.stringify(metadata));
    }

    // 发送请求
    const response = await this.axiosInstance.post('/api/v1/files', formData, {
      headers: {
        ...formData.getHeaders()
      }
    });

    return {
      fileId: response.data.file_id,
      success: response.data.success,
      message: response.data.message
    };
  }

  /**
   * 下载文件
   * 
   * @param fileId 文件ID
   * @returns 文件内容流和元数据
   */
  public async downloadFile(fileId: string): Promise<{
    stream: Readable;
    filename: string;
    contentType: string;
    size: number;
  }> {
    const response = await this.axiosInstance.get(`/api/v1/files/${fileId}`, {
      responseType: 'stream'
    });

    const contentDisposition = response.headers['content-disposition'];
    const filename = contentDisposition ? 
      contentDisposition.split('filename=')[1]?.replace(/"/g, '') || `file-${fileId}` : 
      `file-${fileId}`;

    return {
      stream: response.data as Readable,
      filename,
      contentType: response.headers['content-type'] || 'application/octet-stream',
      size: parseInt(response.headers['content-length'] || '0', 10)
    };
  }

  /**
   * 删除文件
   * 
   * @param fileId 文件ID
   * @returns 删除响应
   */
  public async deleteFile(fileId: string): Promise<DeleteFileResponse> {
    const response = await this.axiosInstance.delete(`/api/v1/files/${fileId}`);
    
    return {
      success: response.data.success,
      message: response.data.message
    };
  }

  /**
   * 获取文件状态
   * 
   * @param fileId 文件ID
   * @returns 文件状态
   */
  public async getFileStatus(fileId: string): Promise<FileStatusResponse> {
    const response = await this.axiosInstance.get(`/api/v1/files/${fileId}/status`);
    
    return {
      fileId: response.data.file_id,
      filename: response.data.filename,
      size: response.data.size,
      status: response.data.status,
      metadata: response.data.metadata || {},
      chunks: response.data.chunks || []
    };
  }

  /**
   * 关闭客户端
   */
  public close(): void {
    // 关闭相关资源
  }
} 