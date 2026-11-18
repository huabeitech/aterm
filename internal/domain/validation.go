package domain

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxDirectoryNameLength = 50

var (
	ErrHostDisplayNameRequired        = errors.New("host display name is required")
	ErrHostDisplayNameInvalid         = errors.New("host display name cannot contain control characters")
	ErrAliasRequired                  = errors.New("alias is required")
	ErrAliasWhitespace                = errors.New("alias cannot contain whitespace")
	ErrHostNameRequired               = errors.New("host name is required")
	ErrPortInvalid                    = errors.New("port must be between 1 and 65535")
	ErrDirectoryNameRequired          = errors.New("directory name is required")
	ErrDirectoryNameTooLong           = errors.New("directory name must be 50 characters or fewer")
	ErrDirectoryNameInvalidCharacters = errors.New("directory name cannot contain / \\ : * ? \" < > | or control characters")
	ErrDirectoryParentCycle           = errors.New("directory cannot be moved into itself or a descendant")
	ErrDirectoryNotEmpty              = errors.New("directory contains hosts")
	ErrHostNotFound                   = errors.New("host not found")
)

func NormalizeHostInput(input HostInput) HostInput {
	input.DirectoryID = strings.TrimSpace(input.DirectoryID)
	input.Name = strings.TrimSpace(input.Name)
	input.Alias = strings.TrimSpace(input.Alias)
	input.HostName = strings.TrimSpace(input.HostName)
	input.User = strings.TrimSpace(input.User)
	input.IdentityFile = strings.TrimSpace(input.IdentityFile)
	input.ProxyJump = strings.TrimSpace(input.ProxyJump)
	input.Remark = strings.TrimSpace(input.Remark)
	if input.Port == 0 {
		input.Port = 22
	}
	return input
}

func ValidateHostInput(input HostInput) error {
	input = NormalizeHostInput(input)
	if input.Name == "" {
		return ErrHostDisplayNameRequired
	}
	if containsControlCharacter(input.Name) {
		return ErrHostDisplayNameInvalid
	}
	if strings.ContainsAny(input.Alias, " \t\r\n") {
		return ErrAliasWhitespace
	}
	if input.HostName == "" {
		return ErrHostNameRequired
	}
	if input.Port < 1 || input.Port > 65535 {
		return ErrPortInvalid
	}
	return nil
}

func NormalizeDirectoryInput(input DirectoryInput) DirectoryInput {
	input.ParentID = strings.TrimSpace(input.ParentID)
	input.Name = strings.TrimSpace(input.Name)
	return input
}

func ValidateDirectoryInput(input DirectoryInput) error {
	input = NormalizeDirectoryInput(input)
	if input.Name == "" {
		return ErrDirectoryNameRequired
	}
	if utf8.RuneCountInString(input.Name) > MaxDirectoryNameLength {
		return ErrDirectoryNameTooLong
	}
	if strings.ContainsAny(input.Name, `/\:*?"<>|`) || containsControlCharacter(input.Name) {
		return ErrDirectoryNameInvalidCharacters
	}
	return nil
}

func containsControlCharacter(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}
