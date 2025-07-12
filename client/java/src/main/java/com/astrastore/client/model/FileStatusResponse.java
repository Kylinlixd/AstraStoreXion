package com.astrastore.client.model;

import com.fasterxml.jackson.annotation.JsonProperty;
import java.util.Map;

/**
 * 文件状态响应
 */
public class FileStatusResponse {
    @JsonProperty("file_id")
    private String fileId;
    
    @JsonProperty("file_name")
    private String fileName;
    
    @JsonProperty("file_size")
    private long fileSize;
    
    @JsonProperty("status")
    private String status;
    
    @JsonProperty("created_at")
    private String createdAt;
    
    @JsonProperty("modified_at")
    private String modifiedAt;
    
    @JsonProperty("checksum")
    private String checksum;
    
    @JsonProperty("storage_nodes")
    private String[] storageNodes;
    
    @JsonProperty("replicas")
    private int replicas;
    
    @JsonProperty("metadata")
    private Map<String, String> metadata;

    // Getters and Setters
    public String getFileId() {
        return fileId;
    }

    public void setFileId(String fileId) {
        this.fileId = fileId;
    }

    public String getFileName() {
        return fileName;
    }

    public void setFileName(String fileName) {
        this.fileName = fileName;
    }

    public long getFileSize() {
        return fileSize;
    }

    public void setFileSize(long fileSize) {
        this.fileSize = fileSize;
    }

    public String getStatus() {
        return status;
    }

    public void setStatus(String status) {
        this.status = status;
    }

    public String getCreatedAt() {
        return createdAt;
    }

    public void setCreatedAt(String createdAt) {
        this.createdAt = createdAt;
    }

    public String getModifiedAt() {
        return modifiedAt;
    }

    public void setModifiedAt(String modifiedAt) {
        this.modifiedAt = modifiedAt;
    }

    public String getChecksum() {
        return checksum;
    }

    public void setChecksum(String checksum) {
        this.checksum = checksum;
    }

    public String[] getStorageNodes() {
        return storageNodes;
    }

    public void setStorageNodes(String[] storageNodes) {
        this.storageNodes = storageNodes;
    }

    public int getReplicas() {
        return replicas;
    }

    public void setReplicas(int replicas) {
        this.replicas = replicas;
    }

    public Map<String, String> getMetadata() {
        return metadata;
    }

    public void setMetadata(Map<String, String> metadata) {
        this.metadata = metadata;
    }
} 