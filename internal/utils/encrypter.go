package utils

type PrivateKeyEncrypter interface {
	Encrypt(message string, passphrase string) (string, error)
	Decrypt(message string, passphrase string) (string, error)
}

type DefaultPrivateKeyEncrypter struct{}

func (e *DefaultPrivateKeyEncrypter) Encrypt(message, passphrase string) (string, error) {
	return Encrypt(message, passphrase)
}

func (e *DefaultPrivateKeyEncrypter) Decrypt(message, passphrase string) (string, error) {
	return Decrypt(message, passphrase)
}

// Making sure that DefaultPrivateKeyEncrypter implements PrivateKeyEncrypter
var _ PrivateKeyEncrypter = (*DefaultPrivateKeyEncrypter)(nil)
