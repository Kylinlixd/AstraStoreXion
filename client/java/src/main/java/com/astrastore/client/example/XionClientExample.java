package com.astrastore.client.example;

import com.astrastore.client.XionClient;
import com.astrastore.client.XionConfig;
import com.astrastore.client.model.FileStatusResponse;
import com.astrastore.client.model.UploadFileResponse;

import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.IOException;
import java.util.HashMap;
import java.util.Map;

/**
 * 星辰离子X Java客户端示例
 */
public class XionClientExample {

    public static void main(String[] args) {
        try {
            // 创建客户端配置
            XionConfig config = new XionConfig("http://localhost:8080", 30000, 3);
            
            // 创建客户端实例
            try (XionClient client = new XionClient(config)) {
                // 上传文件示例
                uploadFileExample(client);
                
                // 下载文件示例
                downloadFileExample(client);
                
                // 获取文件状态示例
                getFileStatusExample(client);
                
                // 删除文件示例
                deleteFileExample(client);
            }
            
            System.out.println("示例完成");
        } catch (Exception e) {
            System.err.println("发生错误: " + e.getMessage());
            e.printStackTrace();
        }
    }
    
    /**
     * 上传文件示例
     */
    private static void uploadFileExample(XionClient client) throws IOException {
        System.out.println("=== 上传文件示例 ===");
        
        // 准备上传文件
        File testFile = createTempFile("test-upload.txt", "这是一个测试文件内容");
        
        // 准备元数据
        Map<String, String> metadata = new HashMap<>();
        metadata.put("description", "测试文件");
        metadata.put("category", "example");
        
        // 上传文件
        try (FileInputStream fis = new FileInputStream(testFile)) {
            UploadFileResponse response = client.uploadFile(fis, testFile.getName(), metadata);
            System.out.println("上传结果: " + response.isSuccess());
            System.out.println("文件ID: " + response.getFileId());
            System.out.println("文件大小: " + response.getFileSize() + " 字节");
            System.out.println("创建时间: " + response.getCreatedAt());
            
            // 保存文件ID以供后续示例使用
            currentFileId = response.getFileId();
        } finally {
            testFile.delete();
        }
    }
    
    /**
     * 下载文件示例
     */
    private static void downloadFileExample(XionClient client) throws IOException {
        System.out.println("\n=== 下载文件示例 ===");
        
        if (currentFileId == null) {
            System.out.println("没有可用的文件ID，跳过下载示例");
            return;
        }
        
        // 准备下载目标
        File downloadedFile = new File("downloaded-" + currentFileId + ".txt");
        
        // 下载文件
        try (FileOutputStream fos = new FileOutputStream(downloadedFile)) {
            client.downloadFile(currentFileId, fos);
            System.out.println("文件已下载到: " + downloadedFile.getAbsolutePath());
            System.out.println("文件大小: " + downloadedFile.length() + " 字节");
        } finally {
            downloadedFile.delete();
        }
    }
    
    /**
     * 获取文件状态示例
     */
    private static void getFileStatusExample(XionClient client) throws IOException {
        System.out.println("\n=== 获取文件状态示例 ===");
        
        if (currentFileId == null) {
            System.out.println("没有可用的文件ID，跳过状态查询示例");
            return;
        }
        
        // 获取文件状态
        FileStatusResponse status = client.getFileStatus(currentFileId);
        System.out.println("文件ID: " + status.getFileId());
        System.out.println("文件名: " + status.getFileName());
        System.out.println("文件大小: " + status.getFileSize() + " 字节");
        System.out.println("状态: " + status.getStatus());
        System.out.println("创建时间: " + status.getCreatedAt());
        System.out.println("修改时间: " + status.getModifiedAt());
        System.out.println("校验和: " + status.getChecksum());
        System.out.println("副本数: " + status.getReplicas());
        
        // 打印元数据
        System.out.println("元数据: ");
        if (status.getMetadata() != null) {
            for (Map.Entry<String, String> entry : status.getMetadata().entrySet()) {
                System.out.println("  " + entry.getKey() + ": " + entry.getValue());
            }
        }
    }
    
    /**
     * 删除文件示例
     */
    private static void deleteFileExample(XionClient client) throws IOException {
        System.out.println("\n=== 删除文件示例 ===");
        
        if (currentFileId == null) {
            System.out.println("没有可用的文件ID，跳过删除示例");
            return;
        }
        
        // 删除文件
        client.deleteFile(currentFileId);
        System.out.println("文件已删除: " + currentFileId);
    }
    
    // 保存当前测试的文件ID
    private static String currentFileId;
    
    /**
     * 创建临时测试文件
     */
    private static File createTempFile(String fileName, String content) throws IOException {
        File tempFile = File.createTempFile("xion-test-", fileName);
        try (FileOutputStream fos = new FileOutputStream(tempFile)) {
            fos.write(content.getBytes());
        }
        return tempFile;
    }
} 