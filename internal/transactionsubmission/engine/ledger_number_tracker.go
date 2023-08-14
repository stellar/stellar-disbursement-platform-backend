package engine

import (
	"fmt"
	"sync"
	"time"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/txnbuild"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
)

const (
	MaxLedgerAge                = 10 * time.Second
	IncrementForMaxLedgerBounds = 10
)

// LedgerNumberTracker is a helper struct that keeps track of the current ledger number.
//
//go:generate mockery --name=LedgerNumberTracker --case=underscore --structname=MockLedgerNumberTracker
type LedgerNumberTracker interface {
	GetLedgerNumber() (int, error)
	GetLedgerBounds() (*txnbuild.LedgerBounds, error)
}

type DefaultLedgerNumberTracker struct {
	maxLedgerAge  time.Duration
	hClient       horizonclient.ClientInterface
	ledgerNumber  int
	lastUpdatedAt time.Time
	// mutex is used to make sure only one call to getLedgerNumberFromHorizon() is running at a time and to prevent running it too often.
	mutex sync.Mutex
}

func NewLedgerNumberTracker(hClient horizonclient.ClientInterface) (*DefaultLedgerNumberTracker, error) {
	if hClient == nil {
		return nil, fmt.Errorf("horizon client cannot be nil")
	}

	return &DefaultLedgerNumberTracker{
		hClient:      hClient,
		maxLedgerAge: MaxLedgerAge,
	}, nil
}

func (se *DefaultLedgerNumberTracker) GetLedgerNumber() (int, error) {
	se.mutex.Lock()
	defer se.mutex.Unlock()

	if time.Since(se.lastUpdatedAt) > se.maxLedgerAge {
		ledgerNumber, err := se.getLedgerNumberFromHorizon()
		if err != nil {
			return 0, fmt.Errorf("getting ledger number from horizon: %w", err)
		} else {
			se.ledgerNumber = ledgerNumber
			se.lastUpdatedAt = time.Now()
		}
	}

	return se.ledgerNumber, nil
}

func (se *DefaultLedgerNumberTracker) getLedgerNumberFromHorizon() (int, error) {
	ledger, err := se.hClient.Root()
	if err != nil {
		return 0, utils.NewHorizonErrorWrapper(err)
	}

	return int(ledger.HorizonSequence), nil
}

func (se *DefaultLedgerNumberTracker) GetLedgerBounds() (*txnbuild.LedgerBounds, error) {
	ledgerNumber, err := se.GetLedgerNumber()
	if err != nil {
		return nil, fmt.Errorf("getting ledger number: %w", err)
	}

	return &txnbuild.LedgerBounds{
		MaxLedger: uint32(ledgerNumber + IncrementForMaxLedgerBounds),
	}, nil
}

var _ LedgerNumberTracker = (*DefaultLedgerNumberTracker)(nil)
