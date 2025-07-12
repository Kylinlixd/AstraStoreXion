package com.astrastore.client;

import com.astrastore.client.model.*;
import com.fasterxml.jackson.databind.ObjectMapper;
import okhttp3.*;

import java.io.Closeable;
import java.io.File;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.URL;
import java.nio.file.Files;
import java.nio.file.StandardCopyOption;
import java.util.Map;
import java.util.concurrent.TimeUnit;

/**
 * 星辰离子X Java客户端
 */
public class XionClient implements Closeable {

    private final XionConfig config;
    private final OkHttpClient httpClient;
    private final ObjectMapper objectMapper;
    private static final MediaType JSON = MediaType.parse("application/json; charset=utf-8");

    /**
     * 创建一个新的客户端实例
     *
     * @param config 客户端配置
     */
    public XionClient(XionConfig config) {
        this.config = config;
        this.objectMapper = new ObjectMapper();
        
        // 创建HTTP客户端
        this.httpClient = new OkHttpClient.Builder()
                .connectTimeout(config.getTimeout(), TimeUnit.MILLISECONDS)
                .readTimeout(config.getTimeout(), TimeUnit.MILLISECONDS)
                .writeTimeout(config.getTimeout(), TimeUnit.MILLISECONDS)
                .build();
    }

    /**
     * 上传文件
     *
     * @param fileInputStream 文件输入流
     * @param filename 文件名
     * @param metadata 元数据
     * @return 上传响应
     * @throws IOException 如果发生IO错误
     */
    public UploadFileResponse uploadFile(InputStream fileInputStream, String filename, Map<String, String> metadata) throws IOException {
        // 创建临时文件
        File tempFile = File.createTempFile("upload-", "-" + filename);
        try {
            // 将输入流复制到临时文件
            Files.copy(fileInputStream, tempFile.toPath(), StandardCopyOption.REPLACE_EXISTING);
            
            // 创建MultipartBody
            MultipartBody.Builder builder = new MultipartBody.Builder()
                    .setType(MultipartBody.FORM)
                    .addFormDataPart("file", filename,
                            RequestBody.create(MediaType.parse("application/octet-stream"), tempFile));
            
            // 添加元数据
            if (metadata != null && !metadata.isEmpty()) {
                String metadataJson = objectMapper.writeValueAsString(metadata);
                builder.addFormDataPart("metadata", metadataJson);
            }
            
            // 构建请求
            Request request = new Request.Builder()
                    .url(config.getApiGateway() + "/api/v1/files")
                    .post(builder.build())
                    .build();
            
            // 执行请求
            try (Response response = httpClient.newCall(request).execute()) {
                if (!response.isSuccessful()) {
                    throw new IOException("上传失败，HTTP状态码: " + response.code());
                }
                
                // 解析响应
                return objectMapper.readValue(response.body().string(), UploadFileResponse.class);
            }
        } finally {
            // 删除临时文件
            tempFile.delete();
        }
    }

    /**
     * 下载文件
     *
     * @param fileId 文件ID
     * @param outputStream 输出流
     * @throws IOException 如果发生IO错误
     */
    public void downloadFile(String fileId, OutputStream outputStream) throws IOException {
        // 构建请求
        Request request = new Request.Builder()
                .url(config.getApiGateway() + "/api/v1/files/" + fileId)
                .get()
                .build();
        
        // 执行请求
        try (Response response = httpClient.newCall(request).execute()) {
            if (!response.isSuccessful()) {
                throw new IOException("下载失败，HTTP状态码: " + response.code());
            }
            
            // 将响应体写入输出流
            byte[] buffer = new byte[config.getChunkSize()];
            int bytesRead;
            InputStream inputStream = response.body().byteStream();
            while ((bytesRead = inputStream.read(buffer)) != -1) {
                outputStream.write(buffer, 0, bytesRead);
            }
            outputStream.flush();
        }
    }

    /**
     * 删除文件
     *
     * @param fileId 文件ID
     * @return 删除响应
     * @throws IOException 如果发生IO错误
     */
    public DeleteFileResponse deleteFile(String fileId) throws IOException {
        // 构建请求
        Request request = new Request.Builder()
                .url(config.getApiGateway() + "/api/v1/files/" + fileId)
                .delete()
                .build();
        
        // 执行请求
        try (Response response = httpClient.newCall(request).execute()) {
            if (!response.isSuccessful()) {
                throw new IOException("删除失败，HTTP状态码: " + response.code());
            }
            
            // 解析响应
            return objectMapper.readValue(response.body().string(), DeleteFileResponse.class);
        }
    }

    /**
     * 获取文件状态
     *
     * @param fileId 文件ID
     * @return 文件状态响应
     * @throws IOException 如果发生IO错误
     */
    public FileStatusResponse getFileStatus(String fileId) throws IOException {
        // 构建请求
        Request request = new Request.Builder()
                .url(config.getApiGateway() + "/api/v1/files/" + fileId + "/status")
                .get()
                .build();
        
        // 执行请求
        try (Response response = httpClient.newCall(request).execute()) {
            if (!response.isSuccessful()) {
                throw new IOException("获取状态失败，HTTP状态码: " + response.code());
            }
            
            // 解析响应
            return objectMapper.readValue(response.body().string(), FileStatusResponse.class);
        }
    }

    /**
     * 关闭客户端
     */
    @Override
    public void close() {
        // 关闭相关资源
    }

    /**
     * 从YAML文件加载配置
     * 
     * @param path 配置文件路径
     * @return 客户端配置
     * @throws IOException 如果发生IO错误
     */
    public static XionConfig fromYamlFile(String path) throws IOException {
        // 实现配置加载逻辑
        // 此处简化实现，实际项目中应使用YAML解析库
        return new XionConfig("http://localhost:8080", 30000, 3);
    }
} 