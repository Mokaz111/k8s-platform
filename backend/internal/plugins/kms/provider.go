package kms

import (
	"context"
)

// MVP_TODO: IKMSProvider 密钥管理插件接口（设计文档 Section 8.4）
// 支持云厂商 KMS：阿里云 KMS、腾讯云 KMS、AWS KMS、Vault、本地 HSM 等
type IKMSProvider interface {
	ProviderType() string

	Name() string

	Init(ctx context.Context, config map[string]interface{}) error

	Encrypt(ctx context.Context, plaintext []byte, aad []byte, keyID string) (*CiphertextBlob, error)

	Decrypt(ctx context.Context, blob *CiphertextBlob, aad []byte) ([]byte, error)

	ReEncrypt(ctx context.Context, blob *CiphertextBlob, newKeyID string, aad []byte) (*CiphertextBlob, error)

	GenerateDataKey(ctx context.Context, keyID string, size int) (plaintext []byte, blob *CiphertextBlob, err error)

	ListKeyVersions(ctx context.Context, keyID string) ([]*KeyVersion, error)

	RotateKey(ctx context.Context, keyID string) (*KeyVersion, error)

	DescribeKey(ctx context.Context, keyID string) (*KeyInfo, error)

	Sign(ctx context.Context, keyID string, message []byte, algorithm string) ([]byte, error)

	Verify(ctx context.Context, keyID string, message, signature []byte, algorithm string) (bool, error)

	Close() error
}

type CiphertextBlob struct {
	KeyID      string
	KeyVersion string
	IV         []byte
	Ciphertext []byte
	Tag        []byte
	Algorithm  string
	Provider   string
	ExtraMeta  map[string]string
}

type KeyVersion struct {
	ID        string
	KeyID     string
	Version   string
	Status    string
	CreatedAt int64
}

type KeyInfo struct {
	KeyID       string
	Name        string
	Description string
	Status      string
	Algorithm   string
	KeyUsage    string
	CreatedAt   int64
	RotationDays int
	Versions    []*KeyVersion
}
