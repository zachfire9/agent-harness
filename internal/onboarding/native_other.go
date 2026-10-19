//go:build !windows

package onboarding

import (
	"errors"
	"os"
)

func terminal(*os.File) bool { return false }
func disableEcho(*os.File) (func() error, error) {
	return nil, errors.New("hidden Windows terminal input required")
}
func NativeStore() CredentialStore { return unavailableStore{} }

type unavailableStore struct{}

func (unavailableStore) Get(string) (string, error) {
	return "", errors.New("Windows Credential Manager unavailable")
}
func (unavailableStore) Put(string, string) error {
	return errors.New("Windows Credential Manager unavailable")
}
func (unavailableStore) Delete(string) error {
	return errors.New("Windows Credential Manager unavailable")
}
