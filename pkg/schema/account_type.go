package schema

type AccountType string

const (
	HostStellarEnv                    AccountType = "HOST.STELLAR.ENV"
	ChannelAccountStellarDB           AccountType = "CHANNEL_ACCOUNT.STELLAR.DB"
	DistributionAccountStellarEnv     AccountType = "DISTRIBUTION_ACCOUNT.STELLAR.ENV"      // was "ENV_STELLAR"
	DistributionAccountStellarDBVault AccountType = "DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT" // was "DB_VAULT_STELLAR"
	DistributionAccountCircleDBVault  AccountType = "DISTRIBUTION_ACCOUNT.CIRCLE.DB_VAULT"  // was "DB_VAULT_CIRCLE"
)

func (t AccountType) IsStellar() bool {
	return t.Platform() == StellarPlatform
}

func (t AccountType) IsCircle() bool {
	return t.Platform() == CirclePlatform
}

// Role represents the role of an account in the system.
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

// StorageMethod represents the method used to store the account secret.
type StorageMethod string

const (
	EnvStorageMethod     StorageMethod = "ENV"
	DBStorageMethod      StorageMethod = "DB"
	DBVaultStorageMethod StorageMethod = "DB_VAULT"
)

var accStorageMethodMap = map[AccountType]StorageMethod{
	HostStellarEnv:                    EnvStorageMethod,
	ChannelAccountStellarDB:           DBStorageMethod,
	DistributionAccountStellarEnv:     DBVaultStorageMethod,
	DistributionAccountStellarDBVault: DBVaultStorageMethod,
	DistributionAccountCircleDBVault:  DBVaultStorageMethod,
}

func (t AccountType) StorageMethod() StorageMethod {
	return accStorageMethodMap[t]
}

// Platform represents the platform where the account is used.
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
