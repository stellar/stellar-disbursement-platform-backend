package dependencyinjection

import "sync"

// dependenciesStore is the global store for all the service instances.
// sync.Map is safe for concurrent use by multiple goroutines without additional locking or coordination.
var dependenciesStore sync.Map

// SetInstance adds a new service instance to the store.
func SetInstance(instanceName string, instance interface{}) {
	dependenciesStore.Store(instanceName, instance)
}

// GetInstance retrieves a service instance by name from the store.
func GetInstance(instanceName string) (interface{}, bool) {
	return dependenciesStore.Load(instanceName)
}
