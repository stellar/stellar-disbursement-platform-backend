package data

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

const (
	APIKeyPrefix     = "SDP_"
	APIKeySaltSize   = 16
	APIKeySecretSize = 32
	maxAttempts      = 3
)

// alphabet is the allowed character set for the keygen
const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

type APIKeyPermission string

const (
	// General
	ReadAll  APIKeyPermission = "read:all"
	WriteAll APIKeyPermission = "write:all"

	// Disbursements
	ReadDisbursements  APIKeyPermission = "read:disbursements"
	WriteDisbursements APIKeyPermission = "write:disbursements"

	// Receivers
	ReadReceivers  APIKeyPermission = "read:receivers"
	WriteReceivers APIKeyPermission = "write:receivers"

	// Payments
	ReadPayments  APIKeyPermission = "read:payments"
	WritePayments APIKeyPermission = "write:payments"

	// Organization
	ReadOrganization  APIKeyPermission = "read:organization"
	WriteOrganization APIKeyPermission = "write:organization"

	// Users
	ReadUsers  APIKeyPermission = "read:users"
	WriteUsers APIKeyPermission = "write:users"

	// Wallets
	ReadWallets  APIKeyPermission = "read:wallets"
	WriteWallets APIKeyPermission = "write:wallets"

	// Statistics
	ReadStatistics APIKeyPermission = "read:statistics"

	// Exports
	ReadExports APIKeyPermission = "read:exports"
)

// validPermissionsMap is the set of all valid permissions for the validation purposes
var validPermissionsMap = map[APIKeyPermission]struct{}{
	ReadAll:            {},
	WriteAll:           {},
	ReadDisbursements:  {},
	WriteDisbursements: {},
	ReadReceivers:      {},
	WriteReceivers:     {},
	ReadPayments:       {},
	WritePayments:      {},
	ReadOrganization:   {},
	WriteOrganization:  {},
	ReadUsers:          {},
	WriteUsers:         {},
	ReadWallets:        {},
	WriteWallets:       {},
	ReadStatistics:     {},
	ReadExports:        {},
}

type APIKeyPermissions []APIKeyPermission

func (p APIKeyPermissions) Value() (driver.Value, error) {
	arr := make([]string, len(p))
	for i, perm := range p {
		arr[i] = string(perm)
	}
	return pq.StringArray(arr).Value()
}

func (p *APIKeyPermissions) Scan(src any) error {
	var arr pq.StringArray
	if err := arr.Scan(src); err != nil {
		return fmt.Errorf("scanning APIKeyPermissions: %w", err)
	}
	perms := make(APIKeyPermissions, len(arr))
	for i, s := range arr {
		perm := APIKeyPermission(s)
		if _, ok := validPermissionsMap[perm]; !ok {
			return fmt.Errorf("invalid permission from DB (%s)", s)
		}
		perms[i] = perm
	}
	*p = perms
	return nil
}

func ValidatePermissions(perms []APIKeyPermission) error {
	for _, p := range perms {
		if _, ok := validPermissionsMap[p]; !ok {
			return fmt.Errorf("invalid permission (%s)", p)
		}
	}
	return nil
}

// IPList represents a list of IPs/CIDRs
type IPList []string

func (ip IPList) Value() (driver.Value, error) {
	return pq.StringArray(ip).Value()
}

func (ip *IPList) Scan(src any) error {
	var arr pq.StringArray
	if err := arr.Scan(src); err != nil {
		return fmt.Errorf("scanning IPList: %w", err)
	}
	*ip = IPList(arr)
	return nil
}

func ValidateAllowedIPs(ips []string) error {
	for _, ip := range ips {
		if strings.Contains(ip, "/") {
			if _, _, err := net.ParseCIDR(ip); err != nil {
				return fmt.Errorf("invalid CIDR: %s", ip)
			}
		} else {
			if net.ParseIP(ip) == nil {
				return fmt.Errorf("invalid IP: %s", ip)
			}
		}
	}
	return nil
}

type APIKey struct {
	ID          string            `db:"id" json:"id"`
	Name        string            `db:"name" json:"name"`
	KeyHash     string            `db:"key_hash" json:"-"`
	Salt        string            `db:"salt" json:"-"`
	ExpiryDate  *time.Time        `db:"expiry_date" json:"expiry_date,omitempty"`
	Permissions APIKeyPermissions `db:"permissions" json:"permissions"`
	AllowedIPs  IPList            `db:"allowed_ips" json:"allowed_ips,omitempty"`
	CreatedAt   time.Time         `db:"created_at" json:"created_at"`
	CreatedBy   string            `db:"created_by" json:"created_by,omitempty"`
	UpdatedAt   time.Time         `db:"updated_at" json:"updated_at"`
	UpdatedBy   string            `db:"updated_by" json:"updated_by,omitempty"`
	LastUsedAt  *time.Time        `db:"last_used_at" json:"last_used_at,omitempty"`
	Key         string            `db:"-" json:"key,omitempty"`
}

func (a *APIKey) HasPermission(req APIKeyPermission) bool {
	// hierarchy respect and shortcircuit if user has *:all permissions
	if strings.HasPrefix(string(req), "read:") && slices.Contains(a.Permissions, ReadAll) {
		return true
	}
	if strings.HasPrefix(string(req), "write:") && slices.Contains(a.Permissions, WriteAll) {
		return true
	}

	return slices.Contains(a.Permissions, req)
}

func (a *APIKey) IsExpired() bool {
	if a.ExpiryDate == nil {
		return false
	}
	return time.Now().UTC().After(*a.ExpiryDate)
}

// IsAllowedIP checks if an IP falls within AllowedIPs (or none means open)
func (a *APIKey) IsAllowedIP(ipStr string) bool {
	if len(a.AllowedIPs) == 0 {
		return true
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, cidr := range a.AllowedIPs {
		if strings.Contains(cidr, "/") {
			_, netw, err := net.ParseCIDR(cidr)
			if err == nil && netw.Contains(ip) {
				return true
			}
		} else if cidr == ipStr {
			return true
		}
	}
	return false
}

type APIKeyModel struct {
	dbConnectionPool db.DBConnectionPool
}

// Insert creates, stores, and returns a new APIKey (including the raw key once).
func (m *APIKeyModel) Insert(
	ctx context.Context,
	name string,
	permissions []APIKeyPermission,
	allowedIPs []string,
	expiry *time.Time,
	createdBy string,
) (*APIKey, error) {
	var apiKey *APIKey
	if allowedIPs == nil {
		allowedIPs = IPList{}
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		saltBytes := make([]byte, APIKeySaltSize)
		if _, err := rand.Read(saltBytes); err != nil {
			return nil, fmt.Errorf("salt gen: %w", err)
		}

		salt := hex.EncodeToString(saltBytes)
		for i := range saltBytes {
			saltBytes[i] = 0
		}

		secret, err := generateSecret()
		if err != nil {
			return nil, err
		}

		// Compute hash = SHA256(salt || secret)
		h := sha256.New()
		h.Write([]byte(salt))
		h.Write([]byte(secret))
		keyHash := hex.EncodeToString(h.Sum(nil))

		candidate := &APIKey{
			ID:          uuid.New().String(),
			Name:        name,
			KeyHash:     keyHash,
			Salt:        salt,
			ExpiryDate:  expiry,
			Permissions: APIKeyPermissions(permissions),
			AllowedIPs:  IPList(allowedIPs),
			CreatedBy:   createdBy,
			UpdatedBy:   createdBy,
		}

		const q = `
            INSERT INTO api_keys (
                id, name, key_hash, salt,
                expiry_date, permissions, allowed_ips,
                created_by, updated_by
            ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
            RETURNING created_at, updated_at
        `

		row := m.dbConnectionPool.QueryRowxContext(ctx, q,
			candidate.ID, candidate.Name, candidate.KeyHash, candidate.Salt,
			candidate.ExpiryDate, candidate.Permissions, candidate.AllowedIPs,
			candidate.CreatedBy, candidate.UpdatedBy,
		)
		if err := row.Scan(&candidate.CreatedAt, &candidate.UpdatedAt); err != nil {
			var pgErr *pq.Error
			if errors.As(err, &pgErr) && pgErr.Code == "23505" && attempt < maxAttempts {
				// hash collision (unique violation) - retry
				continue
			}
			return nil, fmt.Errorf("insert API key: %w", err)
		}

		candidate.Key = APIKeyPrefix + secret
		apiKey = candidate
		break
	}

	if apiKey == nil {
		return nil, fmt.Errorf("could not generate unique API key after %d attempts", maxAttempts)
	}
	return apiKey, nil
}

func (m *APIKeyModel) GetAll(ctx context.Context, createdBy string) ([]*APIKey, error) {
	apiKeys := []*APIKey{}
	query := `
        SELECT
            id, 
			name,
    		expiry_date, 
			permissions, 
			allowed_ips,
    		created_at, 
			created_by,
    		updated_at, 
			updated_by,
    		last_used_at
        FROM
            api_keys
        WHERE
            created_by = $1
        ORDER BY
            created_at DESC
    `

	err := m.dbConnectionPool.SelectContext(ctx, &apiKeys, query, createdBy)
	if err != nil {
		return nil, fmt.Errorf("selecting api keys: %w", err)
	}

	return apiKeys, nil
}

func (m *APIKeyModel) GetByID(ctx context.Context, id, createdBy string) (*APIKey, error) {
	const q = `
      SELECT
        id, name,
        expiry_date, permissions, allowed_ips,
        created_at, created_by,
        updated_at, updated_by,
        last_used_at
      FROM api_keys
      WHERE id = $1
        AND created_by = $2
      LIMIT 1
    `
	var key APIKey
	err := m.dbConnectionPool.GetContext(ctx, &key, q, id, createdBy)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("get api key by id: %w", err)
	}
	return &key, nil
}

func (m *APIKeyModel) Delete(ctx context.Context, id string, createdBy string) error {
	res, err := m.dbConnectionPool.ExecContext(ctx,
		`DELETE FROM api_keys
           WHERE id = $1
             AND created_by = $2`, id, createdBy,
	)
	if err != nil {
		return fmt.Errorf("delete api key: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrRecordNotFound
	}
	return nil
}

func generateSecret() (string, error) {
	secBytes := make([]byte, APIKeySecretSize)
	if _, err := rand.Read(secBytes); err != nil {
		return "", fmt.Errorf("secret gen: %w", err)
	}
	defer func() {
		for i := range secBytes {
			secBytes[i] = 0
		}
	}()

	out := make([]byte, APIKeySecretSize)
	for i, b := range secBytes {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out), nil
}
