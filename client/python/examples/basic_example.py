#!/usr/bin/env python
# -*- coding: utf-8 -*-

"""
基本使用示例
"""

import os
import sys

# 添加父目录到路径，以便导入包
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), '..')))

from astrastore_xion import XionClient, XionConfig, load_config_from_yaml


def main():
    # 创建示例文件
    with open('example.txt', 'w') as f:
        f.write('这是一个示例文本文件，用于测试星辰离子X的Python客户端。')
    
    print("已创建示例文件: example.txt")
    
    # 创建客户端配置
    config = XionConfig({
        'api_gateway': 'http://localhost:8080',
        'timeout': 30000,  # 30秒
        'max_retries': 3
    })
    
    # 或者从配置文件加载配置
    # 如果有config.yaml文件，可以使用以下代码
    # config = load_config_from_yaml('config.yaml')
    
    print("\n=== 使用Python客户端连接到星辰离子X服务 ===")
    print(f"API网关: {config.api_gateway}")
    print(f"超时设置: {config.timeout}毫秒")
    print(f"最大重试: {config.max_retries}次")
    
    # 创建客户端
    client = XionClient(config)
    
    try:
        print("\n=== 上传文件 ===")
        
        # 准备元数据
        metadata = {
            'content_type': 'text/plain',
            'description': '示例文本文件',
            'created_by': 'Python客户端示例'
        }
        
        # 上传文件
        with open('example.txt', 'rb') as f:
            upload_response = client.upload_file(f, 'example.txt', metadata)
        
        print(f"上传状态: {'成功' if upload_response.success else '失败'}")
        print(f"文件ID: {upload_response.file_id}")
        print(f"消息: {upload_response.message}")
        
        if upload_response.success:
            file_id = upload_response.file_id
            
            print("\n=== 获取文件状态 ===")
            status_response = client.get_file_status(file_id)
            
            print(f"文件ID: {status_response.file_id}")
            print(f"文件名: {status_response.filename}")
            print(f"大小: {status_response.size} 字节")
            print(f"状态: {status_response.status}")
            print("元数据:")
            for key, value in status_response.metadata.items():
                print(f"  {key}: {value}")
            
            print("\n=== 下载文件 ===")
            with open('downloaded_example.txt', 'wb') as f:
                client.download_file(file_id, f)
            
            print("文件已保存为: downloaded_example.txt")
            
            # 验证文件内容
            with open('downloaded_example.txt', 'r') as f:
                content = f.read()
                print(f"文件内容: {content[:50]}...")
            
            print("\n=== 删除文件 ===")
            delete_response = client.delete_file(file_id)
            
            print(f"删除状态: {'成功' if delete_response.success else '失败'}")
            print(f"消息: {delete_response.message}")
    
    except Exception as e:
        print(f"\n发生错误: {e}")
    
    finally:
        # 关闭客户端
        client.close()
        print("\n客户端已关闭")


if __name__ == "__main__":
    main() 