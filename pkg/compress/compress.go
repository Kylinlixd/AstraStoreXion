package compress

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"

	"github.com/golang/snappy"
	"github.com/klauspost/compress/zstd"
)

// 错误定义
var (
	ErrInvalidData      = errors.New("无效的数据")
	ErrInvalidAlgorithm = errors.New("无效的算法")
)

// Algorithm 压缩算法
type Algorithm string

const (
	// Gzip压缩
	GzipAlgorithm Algorithm = "gzip"
	// Snappy压缩
	SnappyAlgorithm Algorithm = "snappy"
	// Zstd压缩
	ZstdAlgorithm Algorithm = "zstd"
)

// Level 压缩级别
type Level int

const (
	// 最快压缩
	FastestLevel Level = 1
	// 默认压缩
	DefaultLevel Level = 5
	// 最佳压缩
	BestLevel Level = 9
)

// Compressor 压缩器接口
type Compressor interface {
	// 压缩数据
	Compress(data []byte) ([]byte, error)

	// 解压数据
	Decompress(data []byte) ([]byte, error)

	// 获取算法名称
	Algorithm() Algorithm
}

// CompressorFactory 压缩器工厂
type CompressorFactory struct{}

// NewCompressor 创建压缩器
func (f *CompressorFactory) NewCompressor(algorithm Algorithm, level Level) (Compressor, error) {
	switch algorithm {
	case GzipAlgorithm:
		return NewGzipCompressor(level)
	case SnappyAlgorithm:
		return NewSnappyCompressor(), nil
	case ZstdAlgorithm:
		return NewZstdCompressor(level)
	default:
		return nil, fmt.Errorf("%w: %s", ErrInvalidAlgorithm, algorithm)
	}
}

// GzipCompressor Gzip压缩器实现
type GzipCompressor struct {
	level Level
}

// NewGzipCompressor 创建Gzip压缩器
func NewGzipCompressor(level Level) (*GzipCompressor, error) {
	// 检查压缩级别
	if level < 1 || level > 9 {
		return nil, fmt.Errorf("无效的压缩级别: %d, 应该在1-9之间", level)
	}

	return &GzipCompressor{
		level: level,
	}, nil
}

// Compress 压缩数据
func (c *GzipCompressor) Compress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, ErrInvalidData
	}

	// 创建缓冲区
	var buf bytes.Buffer

	// 创建Gzip写入器
	gw, err := gzip.NewWriterLevel(&buf, int(c.level))
	if err != nil {
		return nil, err
	}

	// 写入数据
	if _, err := gw.Write(data); err != nil {
		gw.Close()
		return nil, err
	}

	// 关闭写入器
	if err := gw.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// Decompress 解压数据
func (c *GzipCompressor) Decompress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, ErrInvalidData
	}

	// 创建Gzip读取器
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gr.Close()

	// 读取解压数据
	return io.ReadAll(gr)
}

// Algorithm 获取算法名称
func (c *GzipCompressor) Algorithm() Algorithm {
	return GzipAlgorithm
}

// SnappyCompressor Snappy压缩器实现
type SnappyCompressor struct{}

// NewSnappyCompressor 创建Snappy压缩器
func NewSnappyCompressor() *SnappyCompressor {
	return &SnappyCompressor{}
}

// Compress 压缩数据
func (c *SnappyCompressor) Compress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, ErrInvalidData
	}

	// 压缩数据
	return snappy.Encode(nil, data), nil
}

// Decompress 解压数据
func (c *SnappyCompressor) Decompress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, ErrInvalidData
	}

	// 解压数据
	decoded, err := snappy.Decode(nil, data)
	if err != nil {
		return nil, err
	}

	return decoded, nil
}

// Algorithm 获取算法名称
func (c *SnappyCompressor) Algorithm() Algorithm {
	return SnappyAlgorithm
}

// ZstdCompressor Zstd压缩器实现
type ZstdCompressor struct {
	level   Level
	encoder *zstd.Encoder
	decoder *zstd.Decoder
}

// NewZstdCompressor 创建Zstd压缩器
func NewZstdCompressor(level Level) (*ZstdCompressor, error) {
	// 检查压缩级别
	if level < 1 || level > 9 {
		return nil, fmt.Errorf("无效的压缩级别: %d, 应该在1-9之间", level)
	}

	// 创建编码器
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.EncoderLevel(level)))
	if err != nil {
		return nil, err
	}

	// 创建解码器
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		encoder.Close()
		return nil, err
	}

	return &ZstdCompressor{
		level:   level,
		encoder: encoder,
		decoder: decoder,
	}, nil
}

// Compress 压缩数据
func (c *ZstdCompressor) Compress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, ErrInvalidData
	}

	// 压缩数据
	return c.encoder.EncodeAll(data, nil), nil
}

// Decompress 解压数据
func (c *ZstdCompressor) Decompress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, ErrInvalidData
	}

	// 解压数据
	return c.decoder.DecodeAll(data, nil)
}

// Algorithm 获取算法名称
func (c *ZstdCompressor) Algorithm() Algorithm {
	return ZstdAlgorithm
}

// Close 关闭压缩器
func (c *ZstdCompressor) Close() {
	if c.encoder != nil {
		c.encoder.Close()
	}
	if c.decoder != nil {
		c.decoder.Close()
	}
}
