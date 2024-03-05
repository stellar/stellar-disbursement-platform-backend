package dependencyinjection

import (
	"context"
	"io"
	"sync"

	"github.com/stellar/go/support/log"
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

// CleanupInstanceByKey removes a service instance from the store by key and test if it is closeable, in which case, its
// Close() method is called.
func CleanupInstanceByKey(ctx context.Context, instanceName string) {
	m.Lock()
	defer m.Unlock()

	instanceToDelete, ok := dependenciesStore[instanceName]
	if !ok {
		return
	}
	delete(dependenciesStore, instanceName)

	if closeableInstance, ok := instanceToDelete.(io.Closer); ok {
		if err := closeableInstance.Close(); err != nil {
			log.Ctx(ctx).Errorf("error closing instance=%s in CleanupInstanceByKey: %v", instanceName, err)
		}
	}
}

// CleanupInstanceByValue removes a service instance from the store by value and checks if it is closeable, in which
// case, its Close() method is called.
func CleanupInstanceByValue(ctx context.Context, instance interface{}) {
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

		if closeableInstance, ok2 := instanceToDelete.(io.Closer); ok2 {
			if err := closeableInstance.Close(); err != nil {
				log.Ctx(ctx).Errorf("error closing instance=%s in CleanupInstanceByValue: %v", k, err)
			}
		}
	}
}
