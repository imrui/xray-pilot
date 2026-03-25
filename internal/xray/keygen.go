package xray

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"

	"golang.org/x/crypto/curve25519"
)

// GenerateX25519KeyPair 生成 Xray Reality 使用的 x25519 密钥对（Base64 URL 编码）
func GenerateX25519KeyPair() (privateKeyB64, publicKeyB64 string, err error) {
	var privateKey [32]byte
	if _, err = rand.Read(privateKey[:]); err != nil {
		return
	}
	// 按 RFC 7748 规范调整私钥
	privateKey[0] &= 248
	privateKey[31] &= 127
	privateKey[31] |= 64

	var publicKey [32]byte
	curve25519.ScalarBaseMult(&publicKey, &privateKey)

	privateKeyB64 = base64.RawURLEncoding.EncodeToString(privateKey[:])
	publicKeyB64 = base64.RawURLEncoding.EncodeToString(publicKey[:])
	return
}

// GenerateShortID 生成 Reality short_id（8 字节随机 hex）
func GenerateShortID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
