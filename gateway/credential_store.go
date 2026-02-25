package gateway

import "carrier/daemon/credentialstore"

func loadProviderCredential(providerID string) (string, string, bool, error) {
	return credentialstore.LoadProviderCredential(providerID)
}

func saveProviderCredential(providerID, value string) (string, error) {
	return credentialstore.SaveProviderCredential(providerID, value)
}
