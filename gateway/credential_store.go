package gateway

import "carrier/daemon/credentialstore"

func loadProviderCredential(providerID string) (string, string, bool, error) {
	return credentialstore.LoadProviderCredential(providerID)
}

func loadProviderCredentialStatus(providerID string) (string, bool, error) {
	_, backend, ok, err := loadProviderCredential(providerID)
	return backend, ok, err
}

func saveProviderCredential(providerID, value string) (string, error) {
	return credentialstore.SaveProviderCredential(providerID, value)
}
