package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_EncryptAndDecrypt_Success(t *testing.T) {
	message := "SBJZIXEH2VE4VQRMWUSYL3PPIOPPKVR5W3LHIZUV46YB22TAB7H4AGBJ"
	key := "1c4d3e4ec75106e0649825b0941fca423f752756a487847d29bb1a9704d17a70e4bac5d52be1933559bcfb43c7017b61d05f4252063f9135b270e8ea99016c03"

	encrypted, err := Encrypt(message, key)
	require.NoError(t, err)

	decrypted, err := Decrypt(encrypted, key)
	require.NoError(t, err)

	assert.Equal(t, message, decrypted)
}

func Test_EncryptAndDecrypt_AuthenticationFailure(t *testing.T) {
	message := "SBJZIXEH2VE4VQRMWUSYL3PPIOPPKVR5W3LHIZUV46YB22TAB7H4AGBJ"
	encryptKey := "9761343c0518b89d92168804c7d7edfc74da8aef8b498d54873836c47c33641bd76b7bdccef361125c638951998076887c6445f11bd0be40feb7cfd4168857e3"
	decryptKey := "1c4d3e4ec75106e0649825b0941fca423f752756a487847d29bb1a9704d17a70e4bac5d52be1933559bcfb43c7017b61d05f4252063f9135b270e8ea99016c03"

	encrypted, err := Encrypt(message, encryptKey)
	require.NoError(t, err)

	_, err = Decrypt(encrypted, decryptKey)
	require.Error(t, err)
}
