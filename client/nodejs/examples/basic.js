const fs = require('fs');
const path = require('path');
const { XionClient } = require('../dist');

async function main() {
  // 创建客户端实例
  const client = new XionClient({
    apiGateway: 'http://localhost:8080'
  });

  try {
    // 上传文件
    console.log('上传文件...');
    const fileStream = fs.createReadStream(path.join(__dirname, 'example.txt'));
    const uploadResponse = await client.uploadFile(fileStream, 'example.txt', {
      contentType: 'text/plain',
      description: '示例文本文件'
    });
    console.log('上传响应:', uploadResponse);

    if (uploadResponse.success) {
      const fileId = uploadResponse.fileId;

      // 获取文件状态
      console.log(`\n获取文件状态 (${fileId})...`);
      const statusResponse = await client.getFileStatus(fileId);
      console.log('状态响应:', statusResponse);

      // 下载文件
      console.log(`\n下载文件 (${fileId})...`);
      const downloadResponse = await client.downloadFile(fileId);
      
      console.log(`文件名: ${downloadResponse.filename}`);
      console.log(`内容类型: ${downloadResponse.contentType}`);
      console.log(`大小: ${downloadResponse.size} 字节`);
      
      // 将文件保存到磁盘
      const outputPath = path.join(__dirname, 'downloaded-' + downloadResponse.filename);
      const outputStream = fs.createWriteStream(outputPath);
      downloadResponse.stream.pipe(outputStream);
      
      await new Promise((resolve, reject) => {
        outputStream.on('finish', resolve);
        outputStream.on('error', reject);
      });
      
      console.log(`文件已保存到 ${outputPath}`);

      // 删除文件
      console.log(`\n删除文件 (${fileId})...`);
      const deleteResponse = await client.deleteFile(fileId);
      console.log('删除响应:', deleteResponse);
    }
  } catch (error) {
    console.error('发生错误:', error.message);
    if (error.response) {
      console.error('响应数据:', error.response.data);
      console.error('响应状态:', error.response.status);
    }
  } finally {
    // 关闭客户端
    client.close();
  }
}

// 创建示例文件
fs.writeFileSync(path.join(__dirname, 'example.txt'), '这是一个示例文本文件。');

// 运行示例
main().catch(console.error); 