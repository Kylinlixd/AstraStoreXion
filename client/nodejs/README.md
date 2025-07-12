# 星辰离子X NodeJS 客户端

这是星辰离子X分布式文件存储系统的NodeJS客户端SDK。

## 安装

```bash
npm install astrastore-xion-client
```

## 快速开始

```javascript
const { XionClient } = require('astrastore-xion-client');
const fs = require('fs');

async function main() {
  // 创建客户端实例
  const client = new XionClient({
    apiGateway: 'http://localhost:8080'
  });

  try {
    // 上传文件
    const fileStream = fs.createReadStream('example.txt');
    const uploadResponse = await client.uploadFile(fileStream, 'example.txt', {
      contentType: 'text/plain',
      description: '示例文本文件'
    });

    console.log(`文件已上传，ID: ${uploadResponse.fileId}`);

    // 下载文件
    const downloadResponse = await client.downloadFile(uploadResponse.fileId);
    const outputStream = fs.createWriteStream('downloaded-file.txt');
    downloadResponse.stream.pipe(outputStream);

    // 获取文件状态
    const statusResponse = await client.getFileStatus(uploadResponse.fileId);
    console.log('文件状态:', statusResponse);

    // 删除文件
    const deleteResponse = await client.deleteFile(uploadResponse.fileId);
    console.log('文件删除结果:', deleteResponse.success);
  } finally {
    client.close();
  }
}

main().catch(console.error);
```

## 配置

```javascript
const { XionClient, loadConfigFromYaml } = require('astrastore-xion-client');

// 从YAML文件加载配置
const config = loadConfigFromYaml('./xion-config.yaml');
const client = new XionClient(config);

// 或者直接传入配置对象
const client = new XionClient({
  apiGateway: 'http://api.example.com',
  timeout: 60000,
  maxRetries: 5
});
```

## API参考

### 创建客户端

```javascript
const client = new XionClient(options);
```

**选项:**
- `apiGateway`: API网关地址
- `timeout`: 请求超时时间（毫秒）
- `maxRetries`: 最大重试次数
- `retryInterval`: 重试间隔（毫秒）
- `chunkSize`: 分块大小（字节）
- `parallelUploads`: 并行上传数
- `parallelDownloads`: 并行下载数
- `enableCache`: 是否启用缓存
- `cacheTTL`: 缓存有效期（毫秒）
- `cacheMaxSize`: 缓存最大大小（字节）

### 方法

#### 上传文件
```javascript
const response = await client.uploadFile(fileStream, filename, metadata);
```

#### 下载文件
```javascript
const response = await client.downloadFile(fileId);
```

#### 删除文件
```javascript
const response = await client.deleteFile(fileId);
```

#### 获取文件状态
```javascript
const response = await client.getFileStatus(fileId);
```

## 示例

更多示例请参见 [examples](./examples) 目录。

## 许可证

MIT 