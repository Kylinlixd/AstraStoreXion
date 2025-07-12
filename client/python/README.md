# 星辰离子X Python客户端

这是星辰离子X分布式文件存储系统的Python客户端SDK。

## 安装

```bash
pip install astrastore-xion-client
```

## 快速开始

```python
from astrastore_xion import XionClient, XionConfig

# 创建配置
config = XionConfig({
    'api_gateway': 'http://localhost:8080',
    'timeout': 30000,  # 30秒
    'max_retries': 3
})

# 创建客户端
client = XionClient(config)

try:
    # 上传文件
    with open('example.txt', 'rb') as f:
        metadata = {
            'content_type': 'text/plain',
            'description': '示例文本文件'
        }
        upload_response = client.upload_file(f, 'example.txt', metadata)
    
    print(f"文件已上传，ID: {upload_response.file_id}")
    
    # 获取文件状态
    status_response = client.get_file_status(upload_response.file_id)
    print(f"文件状态: {status_response.status}")
    
    # 下载文件
    with open('downloaded_example.txt', 'wb') as f:
        client.download_file(upload_response.file_id, f)
    
    print("文件已下载")
    
    # 删除文件
    delete_response = client.delete_file(upload_response.file_id)
    print(f"文件已删除: {delete_response.success}")
    
finally:
    # 关闭客户端
    client.close()
```

## 配置

你可以通过代码创建配置，也可以从YAML文件加载：

```python
from astrastore_xion import XionConfig, load_config_from_yaml

# 从YAML文件加载配置
config = load_config_from_yaml('config.yaml')

# 或者通过代码创建配置
config = XionConfig({
    'api_gateway': 'http://api.example.com',
    'timeout': 60000,  # 60秒
    'max_retries': 5
})
```

YAML配置文件示例：

```yaml
api_gateway: "http://localhost:8080"
timeout: 30000
max_retries: 3
retry_interval: 1000
chunk_size: 4194304  # 4MB
parallel_uploads: 3
parallel_downloads: 3
enable_cache: true
cache_ttl: 60000
cache_max_size: 104857600  # 100MB
```

## API参考

### XionClient

```python
# 创建客户端
client = XionClient(config=None)  # 如果不传config，会使用默认配置
```

#### 方法

- `upload_file(file, filename, metadata=None)` - 上传文件
- `download_file(file_id, output)` - 下载文件
- `delete_file(file_id)` - 删除文件
- `get_file_status(file_id)` - 获取文件状态
- `close()` - 关闭客户端

### 响应类型

- `UploadFileResponse` - 上传文件响应
  - `file_id` - 文件ID
  - `success` - 是否成功
  - `message` - 消息

- `DeleteFileResponse` - 删除文件响应
  - `success` - 是否成功
  - `message` - 消息

- `FileStatusResponse` - 文件状态响应
  - `file_id` - 文件ID
  - `filename` - 文件名
  - `size` - 文件大小（字节）
  - `status` - 文件状态（"available", "pending", "corrupted"）
  - `metadata` - 文件元数据
  - `chunks` - 文件块信息（可选）

## 示例

更多示例请参见 [examples](./examples) 目录。

## 贡献

欢迎贡献代码！请提交Pull Request或创建Issue。

## 许可证

MIT许可证 