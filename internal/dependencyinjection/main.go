package dependencyinjection

// dependenciesStoreMap var is the global map for all the service instances.
var dependenciesStoreMap map[string]interface{} = map[string]interface{}{}

// setInstance adds a new service instance to instances map.
func setInstance(instanceName string, instance interface{}) {
	dependenciesStoreMap[instanceName] = instance
}
