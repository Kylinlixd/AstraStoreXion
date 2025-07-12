#!/usr/bin/env python
# -*- coding: utf-8 -*-

from typing import Dict, List, Optional
from dataclasses import dataclass


@dataclass
class UploadFileResponse:
    """上传文件响应"""
    file_id: str
    success: bool
    message: str
    
    @classmethod
    def from_dict(cls, data: Dict) -> 'UploadFileResponse':
        """从字典创建实例"""
        return cls(
            file_id=data.get('file_id', ''),
            success=data.get('success', False),
            message=data.get('message', '')
        )


@dataclass
class DeleteFileResponse:
    """删除文件响应"""
    success: bool
    message: str
    
    @classmethod
    def from_dict(cls, data: Dict) -> 'DeleteFileResponse':
        """从字典创建实例"""
        return cls(
            success=data.get('success', False),
            message=data.get('message', '')
        )


@dataclass
class ChunkInfo:
    """块信息"""
    chunk_id: str
    node_id: str
    offset: int
    size: int
    
    @classmethod
    def from_dict(cls, data: Dict) -> 'ChunkInfo':
        """从字典创建实例"""
        return cls(
            chunk_id=data.get('chunk_id', ''),
            node_id=data.get('node_id', ''),
            offset=data.get('offset', 0),
            size=data.get('size', 0)
        )


@dataclass
class FileStatusResponse:
    """文件状态响应"""
    file_id: str
    filename: str
    size: int
    status: str  # "available", "pending", "corrupted"
    metadata: Dict[str, str]
    chunks: Optional[List[ChunkInfo]] = None
    
    @classmethod
    def from_dict(cls, data: Dict) -> 'FileStatusResponse':
        """从字典创建实例"""
        chunks = None
        if 'chunks' in data and data['chunks']:
            chunks = [ChunkInfo.from_dict(chunk) for chunk in data['chunks']]
            
        return cls(
            file_id=data.get('file_id', ''),
            filename=data.get('filename', ''),
            size=data.get('size', 0),
            status=data.get('status', 'corrupted'),
            metadata=data.get('metadata', {}),
            chunks=chunks
        ) 