package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// 错误定义
var (
	ErrInvalidKey       = errors.New("无效的密钥")
	ErrInvalidData      = errors.New("无效的数据")
	ErrInvalidBlockSize = errors.New("无效的块大小")
	ErrInvalidIV        = errors.New("无效的初始化向量")
)

// Encryptor 加密器接口
type Encryptor interface {
	// 加密数据
	Encrypt(data []byte) ([]byte, error)

	// 解密数据
	Decrypt(data []byte) ([]byte, error)
}

// Hasher 哈希器接口
type Hasher interface {
	// 计算哈希
	Hash(data []byte) (string, error)

	// 验证哈希
	Verify(data []byte, hash string) (bool, error)
}

// AESEncryptor AES加密器实现
type AESEncryptor struct {
	key []byte
}

// NewAESEncryptor 创建AES加密器
func NewAESEncryptor(key []byte) (*AESEncryptor, error) {
	// 检查密钥长度
	switch len(key) {
	case 16, 24, 32:
		// 有效的AES密钥长度
	default:
		return nil, fmt.Errorf("%w: 长度必须为16、24或32字节", ErrInvalidKey)
	}

	return &AESEncryptor{
		key: key,
	}, nil
}

// Encrypt 加密数据
func (e *AESEncryptor) Encrypt(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, ErrInvalidData
	}

	// 创建加密块
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, err
	}

	// 创建GCM模式
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// 创建随机数（Nonce）
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// 加密数据
	ciphertext := gcm.Seal(nonce, nonce, data, nil)

	return ciphertext, nil
}

// Decrypt 解密数据
func (e *AESEncryptor) Decrypt(data []byte) ([]byte, error) {
	if len(data) < 12 {
		return nil, ErrInvalidData
	}

	// 创建加密块
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, err
	}

	// 创建GCM模式
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// 提取随机数（Nonce）
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, ErrInvalidData
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]

	// 解密数据
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// SHA256Hasher SHA-256哈希器实现
type SHA256Hasher struct {
	salt []byte
}

// NewSHA256Hasher 创建SHA-256哈希器
func NewSHA256Hasher(salt []byte) *SHA256Hasher {
	return &SHA256Hasher{
		salt: salt,
	}
}

// Hash 计算哈希
func (h *SHA256Hasher) Hash(data []byte) (string, error) {
	if len(data) == 0 {
		return "", ErrInvalidData
	}

	// 将盐与数据合并
	salted := append(data, h.salt...)

	// 计算哈希
	hash := sha256.Sum256(salted)

	// 转换为十六进制字符串
	return hex.EncodeToString(hash[:]), nil
}

// Verify 验证哈希
func (h *SHA256Hasher) Verify(data []byte, hash string) (bool, error) {
	// 计算哈希
	calculated, err := h.Hash(data)
	if err != nil {
		return false, err
	}

	// 比较哈希
	return calculated == hash, nil
}

// SHA512Hasher SHA-512哈希器实现
type SHA512Hasher struct {
	salt []byte
}

// NewSHA512Hasher 创建SHA-512哈希器
func NewSHA512Hasher(salt []byte) *SHA512Hasher {
	return &SHA512Hasher{
		salt: salt,
	}
}

// Hash 计算哈希
func (h *SHA512Hasher) Hash(data []byte) (string, error) {
	if len(data) == 0 {
		return "", ErrInvalidData
	}

	// 将盐与数据合并
	salted := append(data, h.salt...)

	// 计算哈希
	hash := sha512.Sum512(salted)

	// 转换为十六进制字符串
	return hex.EncodeToString(hash[:]), nil
}

// Verify 验证哈希
func (h *SHA512Hasher) Verify(data []byte, hash string) (bool, error) {
	// 计算哈希
	calculated, err := h.Hash(data)
	if err != nil {
		return false, err
	}

	// 比较哈希
	return calculated == hash, nil
}

// EncodeBase64 编码为Base64
func EncodeBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// DecodeBase64 从Base64解码
func DecodeBase64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

// GenerateRandomBytes 生成随机字节
func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
	}

	return b, nil
}

// GenerateRandomKey 生成随机密钥
func GenerateRandomKey(size int) ([]byte, error) {
	// 检查密钥长度
	switch size {
	case 16, 24, 32:
		// 有效的AES密钥长度
	default:
		return nil, fmt.Errorf("%w: 长度必须为16、24或32字节", ErrInvalidKey)
	}

	return GenerateRandomBytes(size)
}
