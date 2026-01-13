//go:build darwin

package backend

import (
	"fmt"

	"github.com/keybase/go-keychain"
)

// loadFromKeychain retrieves data from the macOS Keychain
func loadFromKeychain(service, account string) ([]byte, error) {
	query := keychain.NewItem()
	query.SetSecClass(keychain.SecClassGenericPassword)
	query.SetService(service)
	query.SetAccount(account)
	query.SetMatchLimit(keychain.MatchLimitOne)
	query.SetReturnData(true)

	results, err := keychain.QueryItem(query)
	if err != nil {
		return nil, fmt.Errorf("keychain query failed: %w", err)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no keychain item found for %s/%s", service, account)
	}

	return results[0].Data, nil
}

// saveToKeychain stores data in the macOS Keychain
func saveToKeychain(service, account string, data []byte) error {
	// First try to delete any existing item
	_ = deleteFromKeychain(service, account)

	item := keychain.NewItem()
	item.SetSecClass(keychain.SecClassGenericPassword)
	item.SetService(service)
	item.SetAccount(account)
	item.SetData(data)
	item.SetSynchronizable(keychain.SynchronizableNo)
	item.SetAccessible(keychain.AccessibleWhenUnlocked)

	err := keychain.AddItem(item)
	if err != nil {
		return fmt.Errorf("keychain save failed: %w", err)
	}

	return nil
}

// deleteFromKeychain removes data from the macOS Keychain
func deleteFromKeychain(service, account string) error {
	item := keychain.NewItem()
	item.SetSecClass(keychain.SecClassGenericPassword)
	item.SetService(service)
	item.SetAccount(account)

	err := keychain.DeleteItem(item)
	if err != nil && err != keychain.ErrorItemNotFound {
		return fmt.Errorf("keychain delete failed: %w", err)
	}

	return nil
}
