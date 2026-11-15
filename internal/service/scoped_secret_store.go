package service

// ScopedSecretStore isolates credentials for explicit development data profiles.
// The wrapped store remains OS protected; only account names are prefixed.
func ScopedSecretStore(store PasswordStore, scope string) PasswordStore {
	if scope == "" {
		return store
	}
	return scopedSecretStore{store: store, prefix: scope + ":"}
}

type scopedSecretStore struct {
	store  PasswordStore
	prefix string
}

func (s scopedSecretStore) Get(account string) (string, error) {
	return s.store.Get(s.prefix + account)
}
func (s scopedSecretStore) Set(account, value string) error {
	return s.store.Set(s.prefix+account, value)
}
func (s scopedSecretStore) Delete(account string) error { return s.store.Delete(s.prefix + account) }
