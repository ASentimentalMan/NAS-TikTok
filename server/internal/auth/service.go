package auth

import (
	"crypto/sha256"
	"crypto/subtle"
)

type Service struct {
	accountHash  [32]byte
	passwordHash [32]byte
}

func New(account, password string) *Service {
	return &Service{accountHash: sha256.Sum256([]byte(account)), passwordHash: sha256.Sum256([]byte(password))}
}

func (service *Service) Verify(account, password string) bool {
	accountHash := sha256.Sum256([]byte(account))
	passwordHash := sha256.Sum256([]byte(password))
	accountMatches := subtle.ConstantTimeCompare(accountHash[:], service.accountHash[:])
	passwordMatches := subtle.ConstantTimeCompare(passwordHash[:], service.passwordHash[:])
	return accountMatches&passwordMatches == 1
}
