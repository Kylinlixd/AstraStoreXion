#!/usr/bin/env python
# -*- coding: utf-8 -*-

import yaml
from typing import Dict, Any, Optional


class XionConfig:
    """星辰离子X客户端配置"""

    def __init__(self, config_dict: Optional[Dict[str, Any]] = None):
        """
        初始化配置
        
        Args:
            config_dict: 配置字典
        """
        config_dict = config_dict or {}
        
        # API网关地址
        self.api_gateway = config_dict.get('api_gateway', 'http://localhost:8080')
        
        # 请求超时时间（毫秒）
        self.timeout = config_dict.get('timeout', 30000)
        
        # 最大重试次数
        self.max_retries = config_dict.get('max_retries', 3)
        
        # 重试间隔（毫秒）
        self.retry_interval = config_dict.get('retry_interval', 1000)
        
        # 分块大小（字节）
        self.chunk_size = config_dict.get('chunk_size', 4 * 1024 * 1024)  # 默认4MB
        
        # 并行上传数
        self.parallel_uploads = config_dict.get('parallel_uploads', 3)
        
        # 并行下载数
        self.parallel_downloads = config_dict.get('parallel_downloads', 3)
        
        # 是否启用缓存
        self.enable_cache = config_dict.get('enable_cache', True)
        
        # 缓存有效期（毫秒）
        self.cache_ttl = config_dict.get('cache_ttl', 60000)
        
        # 缓存最大大小（字节）
        self.cache_max_size = config_dict.get('cache_max_size', 100 * 1024 * 1024)  # 默认100MB


def load_config_from_yaml(file_path: str) -> XionConfig:
    """
    从YAML文件加载配置
    
    Args:
        file_path: YAML配置文件路径
        
    Returns:
        XionConfig: 配置对象
    """
    try:
        with open(file_path, 'r') as file:
            config_dict = yaml.safe_load(file)
        return XionConfig(config_dict)
    except Exception as e:
        print(f"加载配置文件失败: {e}")
        return XionConfig() 