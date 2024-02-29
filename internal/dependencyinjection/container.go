package dependencyinjection

import (
	"context"
	"sync"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

var (
	// dependenciesStore is the global store for all the service instances.
	dependenciesStore = make(map[string]interface{})
	// the m sync.Mutex is used to safely access & modify the map above from multiple goroutines.
	m sync.Mutex
)

// SetInstance adds a new service instance to the store.
func SetInstance(instanceName string, instance interface{}) {
	m.Lock()
	defer m.Unlock()
	dependenciesStore[instanceName] = instance
}

// GetInstance retrieves a service instance by name from the store.
func GetInstance(instanceName string) (interface{}, bool) {
	m.Lock()
	defer m.Unlock()
	instance, ok := dependenciesStore[instanceName]
	return instance, ok
}

// DeleteInstanceByKey removes a service instance from the store by key and test if it is a dbConnectionPool, in which
// case, the pool is closed.
func DeleteInstanceByKey(ctx context.Context, instanceName string) {
	m.Lock()
	defer m.Unlock()

	instanceToDelete, ok := dependenciesStore[instanceName]
	if !ok {
		return
	}
	delete(dependenciesStore, instanceName)

	if dbConnectionPool, ok := instanceToDelete.(db.DBConnectionPool); ok {
		_ = db.CloseConnectionPoolIfNeeded(ctx, dbConnectionPool)
	}
}

// DeleteInstance removes a service instance from the store by value and test if it is a dbConnectionPool, in which
// case, the pool is closed.
func DeleteInstanceByValue(ctx context.Context, instance interface{}) {
	m.Lock()
	defer m.Unlock()

	keysToDelete := []string{}
	for k, v := range dependenciesStore {
		if v == instance {
			keysToDelete = append(keysToDelete, k)
		}
	}

	for _, k := range keysToDelete {
		instanceToDelete, ok := dependenciesStore[k]
		if !ok {
			continue
		}
		delete(dependenciesStore, k)

		if dbConnectionPool, ok := instanceToDelete.(db.DBConnectionPool); ok {
			_ = db.CloseConnectionPoolIfNeeded(ctx, dbConnectionPool)
		}
	}
}
