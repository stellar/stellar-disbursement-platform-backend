package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubWalletKeyService records calls so tests can assert dry-run makes no writes.
type stubWalletKeyService struct {
	getErr     error
	rotateErr  error
	getCalls   int
	rotateCall int
}

func (s *stubWalletKeyService) StoreKey(_ context.Context, _, _ string) error { return nil }
func (s *stubWalletKeyService) DeleteKey(_ context.Context, _ string) error   { return nil }

func (s *stubWalletKeyService) GetPrivateKey(_ context.Context, _ string) (string, error) {
	s.getCalls++
	if s.getErr != nil {
		return "", s.getErr
	}
	return "SB...SECRET", nil
}

func (s *stubWalletKeyService) RotateWalletDEK(_ context.Context, _ string) error {
	s.rotateCall++
	return s.rotateErr
}

func Test_rotateWalletDEKExec(t *testing.T) {
	ctx := context.Background()
	const pub = "GABC"

	t.Run("dry-run verifies decryption and writes nothing", func(t *testing.T) {
		svc := &stubWalletKeyService{}
		require.NoError(t, rotateWalletDEKExec(ctx, svc, pub, true))
		assert.Equal(t, 1, svc.getCalls)
		assert.Equal(t, 0, svc.rotateCall, "dry-run must not rotate")
	})

	t.Run("live run rotates and re-verifies", func(t *testing.T) {
		svc := &stubWalletKeyService{}
		require.NoError(t, rotateWalletDEKExec(ctx, svc, pub, false))
		assert.Equal(t, 2, svc.getCalls, "pre- and post-rotation verification")
		assert.Equal(t, 1, svc.rotateCall)
	})

	t.Run("pre-verification failure aborts before any write", func(t *testing.T) {
		svc := &stubWalletKeyService{getErr: errors.New("cipher: message authentication failed")}
		err := rotateWalletDEKExec(ctx, svc, pub, false)
		require.ErrorContains(t, err, "before rotation")
		assert.Equal(t, 0, svc.rotateCall)
	})

	t.Run("rotation failure propagates", func(t *testing.T) {
		svc := &stubWalletKeyService{rotateErr: errors.New("db down")}
		err := rotateWalletDEKExec(ctx, svc, pub, false)
		require.ErrorContains(t, err, "rotating wallet DEK")
	})
}
