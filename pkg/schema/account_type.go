package schema

// AccountType represents the type of an account in the system, in the format of a string that displays it's qualifiers
// in the format of {ROLE}.{PLATFORM}.{STORAGE_METHOD}. For example, "HOST.STELLAR.ENV" represents a host account
// that is used in the Stellar platform and stored in the environment.
type AccountType string

const (
	HostStellarEnv                    AccountType = "HOST.STELLAR.ENV"
	ChannelAccountStellarDB           AccountType = "CHANNEL_ACCOUNT.STELLAR.DB"
	DistributionAccountStellarEnv     AccountType = "DISTRIBUTION_ACCOUNT.STELLAR.ENV"      // was "ENV_STELLAR"
	DistributionAccountStellarDBVault AccountType = "DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT" // was "DB_VAULT_STELLAR"
	DistributionAccountCircleDBVault  AccountType = "DISTRIBUTION_ACCOUNT.CIRCLE.DB_VAULT"  // was "DB_VAULT_CIRCLE"
)

func AllAccountTypes() []AccountType {
	return []AccountType{
		HostStellarEnv,
		ChannelAccountStellarDB,
		DistributionAccountStellarEnv,
		DistributionAccountStellarDBVault,
		DistributionAccountCircleDBVault,
	}
}

func DistributionAccountTypes() []AccountType {
	distAccountTypes := []AccountType{}
	for _, accType := range AllAccountTypes() {
		if accType.Role() == DistributionAccountRole {
			distAccountTypes = append(distAccountTypes, accType)
		}
	}
	return distAccountTypes
}

func (t AccountType) IsStellar() bool {
	return t.Platform() == StellarPlatform
}

func (t AccountType) IsCircle() bool {
	return t.Platform() == CirclePlatform
}

// Role represents the role of an account in the system, e.g. HOST, CHANNEL_ACCOUNT, or DISTRIBUTION_ACCOUNT.
type Role string

const (
	HostRole                Role = "HOST"
	ChannelAccountRole      Role = "CHANNEL_ACCOUNT"
	DistributionAccountRole Role = "DISTRIBUTION_ACCOUNT"
)

var accRoleMap = map[AccountType]Role{
	HostStellarEnv:                    HostRole,
	ChannelAccountStellarDB:           ChannelAccountRole,
	DistributionAccountStellarEnv:     DistributionAccountRole,
	DistributionAccountStellarDBVault: DistributionAccountRole,
	DistributionAccountCircleDBVault:  DistributionAccountRole,
}

func (t AccountType) Role() Role {
	return accRoleMap[t]
}

// Platform represents the platform where the account is used, e.g. STELLAR, or CIRCLE.
type Platform string

const (
	StellarPlatform Platform = "STELLAR"
	CirclePlatform  Platform = "CIRCLE"
)

var accPlatformMap = map[AccountType]Platform{
	HostStellarEnv:                    StellarPlatform,
	ChannelAccountStellarDB:           StellarPlatform,
	DistributionAccountStellarEnv:     StellarPlatform,
	DistributionAccountStellarDBVault: StellarPlatform,
	DistributionAccountCircleDBVault:  CirclePlatform,
}

func (t AccountType) Platform() Platform {
	return accPlatformMap[t]
}

// StorageMethod represents the method used to store the account secret, e.g. ENV, DB_VAULT, or DB.
type StorageMethod string

const (
	EnvStorageMethod     StorageMethod = "ENV"
	DBStorageMethod      StorageMethod = "DB"
	DBVaultStorageMethod StorageMethod = "DB_VAULT"
)

var accStorageMethodMap = map[AccountType]StorageMethod{
	HostStellarEnv:                    EnvStorageMethod,
	ChannelAccountStellarDB:           DBStorageMethod,
	DistributionAccountStellarEnv:     EnvStorageMethod,
	DistributionAccountStellarDBVault: DBVaultStorageMethod,
	DistributionAccountCircleDBVault:  DBVaultStorageMethod,
}

func (t AccountType) StorageMethod() StorageMethod {
	return accStorageMethodMap[t]
}
