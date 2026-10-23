package api

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

const productionPasswordHashCost = bcrypt.DefaultCost

// passwordHashCost is only ever reassigned from _test.go files, so a deployed binary always hashes at the production cost.
var passwordHashCost = productionPasswordHashCost

func hashPassword(password string) ([]byte, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), passwordHashCost)
	if err != nil {
		return nil, fmt.Errorf("api: hash password: %w", err)
	}
	return hash, nil
}
