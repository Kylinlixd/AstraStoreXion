/**
 * 上传文件响应
 */
export interface UploadFileResponse {
  /**
   * 文件ID
   */
  fileId: string;

  /**
   * 是否成功
   */
  success: boolean;

  /**
   * 消息
   */
  message: string;
}

/**
 * 删除文件响应
 */
export interface DeleteFileResponse {
  /**
   * 是否成功
   */
  success: boolean;

  /**
   * 消息
   */
  message: string;
}

/**
 * 块信息
 */
export interface ChunkInfo {
  /**
   * 块ID
   */
  chunkId: string;

  /**
   * 节点ID
   */
  nodeId: string;

  /**
   * 偏移量
   */
  offset: number;

  /**
   * 大小
   */
  size: number;
}

/**
 * 文件状态响应
 */
export interface FileStatusResponse {
  /**
   * 文件ID
   */
  fileId: string;

  /**
   * 文件名
   */
  filename: string;

  /**
   * 大小
   */
  size: number;

  /**
   * 状态
   */
  status: 'available' | 'pending' | 'corrupted';

  /**
   * 元数据
   */
  metadata: Record<string, string>;

  /**
   * 块信息
   */
  chunks?: ChunkInfo[];
} 