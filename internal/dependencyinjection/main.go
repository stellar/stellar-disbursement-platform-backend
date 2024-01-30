package dependencyinjection

import "sync"

var (
	// dependenciesStoreMap var is the global map for all the service instances.
	dependenciesStoreMap = make(map[string]interface{})
	// dependenciesStoreMutex is the mutex for the global map, used to make sure the map is thread safe.
	dependenciesStoreMutex sync.Mutex
)

// SetInstance adds a new service instance to instances map.
func SetInstance(instanceName string, instance interface{}) {
	dependenciesStoreMutex.Lock()
	defer dependenciesStoreMutex.Unlock()

	dependenciesStoreMap[instanceName] = instance
}

// GetInstance retrieves a service instance by name from the instances map.
func GetInstance(instanceName string) (interface{}, bool) {
	dependenciesStoreMutex.Lock()
	defer dependenciesStoreMutex.Unlock()

	instance, ok := dependenciesStoreMap[instanceName]
	return instance, ok
}
