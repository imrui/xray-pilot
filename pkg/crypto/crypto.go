package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/imrui/xray-pilot/config"
)

// masterKeyBytes 返回 32 字节的 AES-256 密钥
func masterKeyBytes() ([]byte, error) {
	key := config.Global.Crypto.MasterKey
	if key == "" {
		return nil, errors.New("MasterKey 未配置")
	}
	b, err := hex.DecodeString(key)
	if err != nil {
		// 非 hex 格式，直接 SHA256 派生
		h := sha256.Sum256([]byte(key))
		return h[:], nil
	}
	if len(b) != 32 {
		return nil, fmt.Errorf("MasterKey 长度应为 32 字节（64位hex），实际 %d 字节", len(b))
	}
	return b, nil
}

// Encrypt 使用 AES-GCM 加密明文，返回 hex(nonce+ciphertext)
func Encrypt(plaintext string) (string, error) {
	key, err := masterKeyBytes()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(ciphertext), nil
}

// Decrypt 解密 Encrypt() 输出的 hex 字符串
func Decrypt(cipherHex string) (string, error) {
	key, err := masterKeyBytes()
	if err != nil {
		return "", err
	}
	data, err := hex.DecodeString(cipherHex)
	if err != nil {
		return "", fmt.Errorf("非法密文格式: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("密文长度不足")
	}
	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("解密失败: %w", err)
	}
	return string(plain), nil
}

// HashConfig 计算配置内容的 SHA256，用于 ConfigHash 漂移检测
func HashConfig(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}
