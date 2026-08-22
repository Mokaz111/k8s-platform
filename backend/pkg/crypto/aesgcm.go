package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/k8s-platform/console/pkg/errcode"
)

// AESGCM AES-256-GCM 加解密实现（KMS Provider 的本地实现）
// 密文格式（二进制）：[1字节版本=0x01][12字节Nonce][密文+16字节Tag]
// 对外统一使用 Base64 字符串存储
type AESGCM struct {
	key        []byte
	keyVersion string
	previous   []byte // 上一版本密钥，用于轮换过渡期解密
}

const (
	aesVersionByte = 0x01
	nonceSize      = 12
)

// NewAESGCM 从 64 字符 hex 字符串（32 字节）创建密钥
func NewAESGCM(keyHex, keyVersion, previousKeyHex string) (*AESGCM, error) {
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid aes key hex: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("aes key must be 32 bytes (256bit), got %d", len(key))
	}

	a := &AESGCM{key: key, keyVersion: keyVersion}
	if previousKeyHex != "" {
		prev, err := hex.DecodeString(previousKeyHex)
		if err != nil {
			return nil, fmt.Errorf("invalid previous aes key: %w", err)
		}
		if len(prev) != 32 {
			return nil, fmt.Errorf("previous aes key must be 32 bytes, got %d", len(prev))
		}
		a.previous = prev
	}
	return a, nil
}

// Encrypt 明文 + AAD → 密文（版本前缀 + Nonce + Ciphertext）
func (a *AESGCM) Encrypt(plaintext, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(a.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce error: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, aad)

	// 拼接: [1B version][12B nonce][ciphertext+tag]
	out := make([]byte, 1+nonceSize+len(ciphertext))
	out[0] = aesVersionByte
	copy(out[1:1+nonceSize], nonce)
	copy(out[1+nonceSize:], ciphertext)
	return out, nil
}

// Decrypt 解密密文。顺序尝试：当前密钥 → 历史密钥。
func (a *AESGCM) Decrypt(cipherblob, aad []byte) ([]byte, error) {
	if len(cipherblob) < 1+nonceSize+16 {
		return nil, errcode.Wrap(errcode.KMSDecryptFail, fmt.Errorf("cipher too short: %d", len(cipherblob)))
	}
	if cipherblob[0] != aesVersionByte {
		return nil, errcode.Wrap(errcode.KMSDecryptFail, fmt.Errorf("unknown version byte: %x", cipherblob[0]))
	}

	nonce := cipherblob[1 : 1+nonceSize]
	ciphertext := cipherblob[1+nonceSize:]

	keysToTry := [][]byte{a.key}
	if a.previous != nil {
		keysToTry = append(keysToTry, a.previous)
	}

	var lastErr error
	for _, k := range keysToTry {
		block, err := aes.NewCipher(k)
		if err != nil {
			lastErr = err
			continue
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			lastErr = err
			continue
		}
		plain, err := gcm.Open(nil, nonce, ciphertext, aad)
		if err == nil {
			return plain, nil
		}
		lastErr = err
	}
	return nil, errcode.Wrap(errcode.KMSDecryptFail, lastErr, "all keys tried")
}

// KeyVersion 返回密钥版本
func (a *AESGCM) KeyVersion() string { return a.keyVersion }

// ZeroBuffer 安全清理敏感字节切片内存（写入 0）
func ZeroBuffer(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
