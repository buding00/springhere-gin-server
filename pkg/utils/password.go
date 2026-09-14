package utils

import "golang.org/x/crypto/bcrypt"

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("springhere-dummy-password"), bcrypt.DefaultCost)

func HashPassword(value string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(value), bcrypt.DefaultCost)
	return string(b), err
}
func Compare(hash, value string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(value))
}
func DummyCompare(value string) { _ = bcrypt.CompareHashAndPassword(dummyHash, []byte(value)) }
