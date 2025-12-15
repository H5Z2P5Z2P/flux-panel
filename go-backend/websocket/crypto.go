package websocket

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

type AESCrypto struct {
	key []byte
}

func NewAESCrypto(secret string) *AESCrypto {
	if secret == "" {
		return nil
	}
	hash := sha256.Sum256([]byte(secret))
	return &AESCrypto{
		key: hash[:],
	}
}

func (ac *AESCrypto) Encrypt(text []byte) (string, error) {
	block, err := aes.NewCipher(ac.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nil, nonce, text, nil)
	encrypted := append(nonce, ciphertext...)

	return base64.StdEncoding.EncodeToString(encrypted), nil
}

func (ac *AESCrypto) Decrypt(text string) ([]byte, error) {
	encrypted, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(ac.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(encrypted) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := encrypted[:nonceSize], encrypted[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func (ac *AESCrypto) DecryptString(text string) (string, error) {
	bytes, err := ac.Decrypt(text)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
