#!/usr/bin/env python
# -*- coding: utf-8 -*-

import os
import time
import json
import requests
from typing import Dict, BinaryIO, Optional, Union, Any

from .config import XionConfig
from .models import UploadFileResponse, DeleteFileResponse, FileStatusResponse


class XionClient:
    """星辰离子X客户端"""

    def __init__(self, config: Optional[XionConfig] = None):
        """
        初始化客户端
        
        Args:
            config: 客户端配置，如果为None则使用默认配置
        """
        self.config = config or XionConfig()
        self.session = requests.Session()
        
        # 设置会话参数
        self.session.headers.update({
            'User-Agent': 'XionClient/Python/1.0.0',
            'Accept': 'application/json',
        })

    def upload_file(
            self, 
            file: Union[BinaryIO, bytes], 
            filename: str, 
            metadata: Optional[Dict[str, str]] = None
        ) -> UploadFileResponse:
        """
        上传文件
        
        Args:
            file: 文件内容或文件对象
            filename: 文件名
            metadata: 元数据，键值对
            
        Returns:
            UploadFileResponse: 上传响应
        """
        files = {'file': (filename, file)}
        
        data = {}
        if metadata:
            data['metadata'] = json.dumps(metadata)
        
        url = f"{self.config.api_gateway}/api/v1/files"
        
        response = self._make_request(
            'POST',
            url,
            files=files,
            data=data
        )
        
        return UploadFileResponse.from_dict(response)

    def download_file(self, file_id: str, output: BinaryIO) -> None:
        """
        下载文件
        
        Args:
            file_id: 文件ID
            output: 输出流
            
        Returns:
            None
        """
        url = f"{self.config.api_gateway}/api/v1/files/{file_id}"
        
        with self.session.get(
            url,
            stream=True,
            timeout=self.config.timeout / 1000  # 转换为秒
        ) as response:
            response.raise_for_status()
            
            # 按块读取并写入
            for chunk in response.iter_content(chunk_size=self.config.chunk_size):
                if chunk:  # 过滤掉keep-alive包
                    output.write(chunk)

    def delete_file(self, file_id: str) -> DeleteFileResponse:
        """
        删除文件
        
        Args:
            file_id: 文件ID
            
        Returns:
            DeleteFileResponse: 删除响应
        """
        url = f"{self.config.api_gateway}/api/v1/files/{file_id}"
        
        response = self._make_request('DELETE', url)
        
        return DeleteFileResponse.from_dict(response)

    def get_file_status(self, file_id: str) -> FileStatusResponse:
        """
        获取文件状态
        
        Args:
            file_id: 文件ID
            
        Returns:
            FileStatusResponse: 文件状态
        """
        url = f"{self.config.api_gateway}/api/v1/files/{file_id}/status"
        
        response = self._make_request('GET', url)
        
        return FileStatusResponse.from_dict(response)

    def close(self) -> None:
        """
        关闭客户端
        
        Returns:
            None
        """
        self.session.close()

    def _make_request(self, method: str, url: str, **kwargs) -> Dict:
        """
        发送请求
        
        Args:
            method: HTTP方法
            url: 请求URL
            **kwargs: 请求参数
            
        Returns:
            Dict: 响应数据
            
        Raises:
            requests.exceptions.HTTPError: HTTP错误
        """
        kwargs.setdefault('timeout', self.config.timeout / 1000)  # 转换为秒
        
        retry_count = 0
        last_exception = None
        
        while retry_count <= self.config.max_retries:
            try:
                response = self.session.request(method, url, **kwargs)
                response.raise_for_status()
                return response.json()
            except (requests.exceptions.RequestException, json.JSONDecodeError) as e:
                last_exception = e
                retry_count += 1
                
                if retry_count <= self.config.max_retries:
                    # 等待后重试
                    time.sleep(self.config.retry_interval / 1000)  # 转换为秒
                    continue
                break
        
        # 重试次数用尽，抛出最后一个异常
        if last_exception:
            raise last_exception
        
        raise requests.exceptions.RequestException("请求失败，原因未知")

    def __enter__(self) -> 'XionClient':
        return self
        
    def __exit__(self, exc_type, exc_val, exc_tb) -> None:
        self.close() 