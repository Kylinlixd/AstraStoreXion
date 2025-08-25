# 星辰离子X API参考文档

本文档详细说明了星辰离子X分布式文件存储系统提供的所有API端点、请求参数、响应格式和认证要求。

## 目录

- [认证与授权](#认证与授权)
- [文件操作](#文件操作)
- [系统管理](#系统管理)
- [错误处理](#错误处理)
- [限流与熔断](#限流与熔断)

## 认证与授权

所有API请求（除了健康检查和登录接口）都需要进行身份验证。身份验证通过JWT令牌实现，令牌可以通过登录API获取。

### 认证方式

在HTTP请求中包含`Authorization`头，值为`Bearer {token}`，其中`{token}`是通过登录API获取的JWT令牌。

例如：
```
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

或者在URL中包含`token`参数：
```
GET /api/v1/files/123?token=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

### 登录

**请求**：
```
POST /api/v1/auth/login
Content-Type: application/json

{
  "username": "user",
  "password": "password"
}
```

**响应**：
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user_id": "user-001",
  "role": "user"
}
```

### 刷新令牌

**请求**：
```
POST /api/v1/auth/refresh
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**响应**：
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

## 文件操作

### 上传文件

**请求**：
```
POST /api/v1/files
Content-Type: multipart/form-data
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...

------WebKitFormBoundary7MA4YWxkTrZu0gW
Content-Disposition: form-data; name="file"; filename="example.txt"
Content-Type: text/plain

文件内容...
------WebKitFormBoundary7MA4YWxkTrZu0gW
Content-Disposition: form-data; name="metadata"

{"description":"示例文件","tags":["示例","文档"]}
------WebKitFormBoundary7MA4YWxkTrZu0gW--
```

**响应**：
```json
{
  "file_id": "file-1621234567890",
  "success": true,
  "message": "文件上传成功"
}
```

### 下载文件

**请求**：
```
GET /api/v1/files/{file_id}
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**响应**：
文件内容，带有适当的`Content-Type`和`Content-Disposition`头。

### 获取文件状态

**请求**：
```
GET /api/v1/files/{file_id}/status
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**响应**：
```json
{
  "file_id": "file-1621234567890",
  "filename": "example.txt",
  "size": 1024,
  "status": "available",
  "metadata": {
    "content_type": "text/plain",
    "created_by": "user",
    "upload_time": "2023-05-18T10:30:45Z",
    "description": "示例文件",
    "tags": ["示例", "文档"]
  }
}
```

### 删除文件

**请求**：
```
DELETE /api/v1/files/{file_id}
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**响应**：
```json
{
  "success": true,
  "message": "文件删除成功"
}
```

## 系统管理

### 健康检查

**请求**：
```
GET /health
```

**响应**：
```json
{
  "status": "ok",
  "timestamp": "2023-05-18T10:30:45Z",
  "version": "1.0.0"
}
```

## 错误处理

当API请求失败时，响应将包含错误信息和适当的HTTP状态码。

**错误响应格式**：
```json
{
  "success": false,
  "error": "错误描述"
}
```

**常见HTTP状态码**：

- `200 OK`: 请求成功
- `400 Bad Request`: 请求参数错误
- `401 Unauthorized`: 未提供认证或认证失败
- `403 Forbidden`: 权限不足
- `404 Not Found`: 资源不存在
- `500 Internal Server Error`: 服务器内部错误

## 限流与熔断

API网关实现了限流和熔断机制，以保护系统免受过载。

### 限流策略

- 每个IP地址每分钟最多100个请求
- 每个用户每分钟最多300个请求
- 上传文件每个用户每分钟最多10个请求

当超过限制时，API将返回`429 Too Many Requests`状态码。

### 熔断策略

当系统检测到某个服务异常率超过阈值时，将触发熔断，暂时拒绝该服务的请求。熔断恢复后，系统将逐步恢复对该服务的请求。

当触发熔断时，API将返回`503 Service Unavailable`状态码。 