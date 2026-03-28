package validator

import (
	"fmt"
	"net/mail"
	"regexp"
)

var (
	isValidUsername = regexp.MustCompile(`^[a-z0-9_]+$`).MatchString
	isValidName     = regexp.MustCompile(`^[a-zA-Z\s]+$`).MatchString
	isValidUUID     = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`).MatchString
)

func ValidateString(value string, minLength int, maxLength int) error {
	n := len(value)
	if n < minLength || n > maxLength {
		return fmt.Errorf("please enter between %d and %d characters", minLength, maxLength)
	}
	return nil
}

func ValidateUsername(value string) error {
	if err := ValidateString(value, 3, 100); err != nil {
		return err
	}
	if !isValidUsername(value) {
		return fmt.Errorf("username must only contain lowercase letters, digits, or underscores")
	}
	return nil
}

func ValidateName(value string) error {
	if err := ValidateString(value, 1, 100); err != nil {
		return err
	}
	if !isValidName(value) {
		return fmt.Errorf("name must only contain letters or spaces")
	}
	return nil
}

func ValidatePassword(value string) error {
	return ValidateString(value, 6, 100)
}

func ValidateEmail(value string) error {
	if err := ValidateString(value, 3, 200); err != nil {
		return err
	}
	if _, err := mail.ParseAddress(value); err != nil {
		return fmt.Errorf("invalid email address")
	}
	return nil
}

func ValidateEmailVerificationToken(value string) error {
	if !isValidUUID(value) {
		return fmt.Errorf("token validation failed")
	}
	return nil
}

func ValidateInt32(value int32) error {
	if value < 0 {
		return fmt.Errorf("value must be a zero or a positive integer")
	}
	return nil
}

func ValidateInt64(value int64) error {
	if value <= 0 {
		return fmt.Errorf("value must be a positive integer")
	}
	return nil
}

func ValidateStringLength(value string) error {
	return ValidateString(value, 32, 128)
}

func ValidateRole(value string) error {
	if value != "admin" && value != "host" && value != "client" {
		return fmt.Errorf("role must be one of 'admin', 'host', or 'client'")
	}
	return nil
}
