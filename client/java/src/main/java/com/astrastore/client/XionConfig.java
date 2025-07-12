package com.astrastore.client;

/**
 * 星辰离子X客户端配置
 */
public class XionConfig {

    private String apiGateway;         // API网关地址
    private int timeout;               // 请求超时时间 (毫秒)
    private int maxRetries;            // 最大重试次数
    private int retryInterval;         // 重试间隔 (毫秒)
    private int chunkSize;             // 分块大小 (字节)
    private int parallelUploads;       // 并行上传数
    private int parallelDownloads;     // 并行下载数
    private boolean enableCache;       // 是否启用缓存
    private int cacheTTL;              // 缓存有效期 (秒)
    private long cacheMaxSize;         // 缓存最大大小 (字节)

    /**
     * 创建一个新的配置实例
     * 
     * @param apiGateway API网关地址
     * @param timeout 超时时间 (毫秒)
     * @param maxRetries 最大重试次数
     */
    public XionConfig(String apiGateway, int timeout, int maxRetries) {
        this.apiGateway = apiGateway;
        this.timeout = timeout;
        this.maxRetries = maxRetries;
        
        // 设置默认值
        this.retryInterval = 1000;
        this.chunkSize = 4 * 1024 * 1024; // 4MB
        this.parallelUploads = 3;
        this.parallelDownloads = 3;
        this.enableCache = true;
        this.cacheTTL = 60;
        this.cacheMaxSize = 100 * 1024 * 1024; // 100MB
    }

    /**
     * 创建一个带有所有参数的新配置实例
     * 
     * @param apiGateway API网关地址
     * @param timeout 超时时间 (毫秒)
     * @param maxRetries 最大重试次数
     * @param retryInterval 重试间隔 (毫秒)
     * @param chunkSize 分块大小 (字节)
     * @param parallelUploads 并行上传数
     * @param parallelDownloads 并行下载数
     * @param enableCache 是否启用缓存
     * @param cacheTTL 缓存有效期 (秒)
     * @param cacheMaxSize 缓存最大大小 (字节)
     */
    public XionConfig(String apiGateway, int timeout, int maxRetries, int retryInterval, 
                      int chunkSize, int parallelUploads, int parallelDownloads, 
                      boolean enableCache, int cacheTTL, long cacheMaxSize) {
        this.apiGateway = apiGateway;
        this.timeout = timeout;
        this.maxRetries = maxRetries;
        this.retryInterval = retryInterval;
        this.chunkSize = chunkSize;
        this.parallelUploads = parallelUploads;
        this.parallelDownloads = parallelDownloads;
        this.enableCache = enableCache;
        this.cacheTTL = cacheTTL;
        this.cacheMaxSize = cacheMaxSize;
    }

    // Getters and Setters
    public String getApiGateway() {
        return apiGateway;
    }

    public void setApiGateway(String apiGateway) {
        this.apiGateway = apiGateway;
    }

    public int getTimeout() {
        return timeout;
    }

    public void setTimeout(int timeout) {
        this.timeout = timeout;
    }

    public int getMaxRetries() {
        return maxRetries;
    }

    public void setMaxRetries(int maxRetries) {
        this.maxRetries = maxRetries;
    }

    public int getRetryInterval() {
        return retryInterval;
    }

    public void setRetryInterval(int retryInterval) {
        this.retryInterval = retryInterval;
    }

    public int getChunkSize() {
        return chunkSize;
    }

    public void setChunkSize(int chunkSize) {
        this.chunkSize = chunkSize;
    }

    public int getParallelUploads() {
        return parallelUploads;
    }

    public void setParallelUploads(int parallelUploads) {
        this.parallelUploads = parallelUploads;
    }

    public int getParallelDownloads() {
        return parallelDownloads;
    }

    public void setParallelDownloads(int parallelDownloads) {
        this.parallelDownloads = parallelDownloads;
    }

    public boolean isEnableCache() {
        return enableCache;
    }

    public void setEnableCache(boolean enableCache) {
        this.enableCache = enableCache;
    }

    public int getCacheTTL() {
        return cacheTTL;
    }

    public void setCacheTTL(int cacheTTL) {
        this.cacheTTL = cacheTTL;
    }

    public long getCacheMaxSize() {
        return cacheMaxSize;
    }

    public void setCacheMaxSize(long cacheMaxSize) {
        this.cacheMaxSize = cacheMaxSize;
    }
} 